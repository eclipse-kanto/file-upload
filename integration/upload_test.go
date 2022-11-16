// Copyright (c) 2022 Contributors to the Eclipse Foundation
//
// See the NOTICE file(s) distributed with this work for additional
// information regarding copyright ownership.
//
// This program and the accompanying materials are made available under the
// terms of the Eclipse Public License 2.0 which is available at
// https://www.eclipse.org/legal/epl-2.0, or the Apache License, Version 2.0
// which is available at https://www.apache.org/licenses/LICENSE-2.0.
//
// SPDX-License-Identifier: EPL-2.0 OR Apache-2.0

//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/caarlos0/env/v6"
	"github.com/eclipse-kanto/file-upload/client"
	"github.com/eclipse-kanto/file-upload/uploaders"
	"github.com/eclipse-kanto/kanto/integration/util"
	"github.com/eclipse/ditto-clients-golang/protocol"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"golang.org/x/net/websocket"
)

type uploadRequest struct {
	CorrelationID string            `json:"correlationId"`
	Options       map[string]string `json:"options"`
}

type uploadStatus struct {
	State    string `json:"state"`
	Progress int    `json:"progress"`
}

func (suite *uploadTestSuite) SetupSuite() {
	suite.initializer = util.SuiteInitializer{}
	suite.initializer.Setup(suite.T())

	opts := env.Options{RequiredIfNoDef: false}
	suite.cfg = uploadTestConfig{}
	require.NoError(suite.T(), env.Parse(&suite.cfg, opts), "failed to process environment variables")
	suite.checkUploadDir()

	thingCfg, err := util.GetThingConfiguration(suite.initializer.Cfg, suite.initializer.MQTTClient)
	require.NoError(suite.T(), err, "failed to get thing configuration")

	suite.thingURL = fmt.Sprintf("%s/api/2/things/%s", strings.TrimSuffix(suite.initializer.Cfg.DigitalTwinAPIAddress, "/"),
		thingCfg.DeviceID)
	suite.featureURL = fmt.Sprintf("%s/features/%s", suite.thingURL, featureID)
}

func (suite *uploadTestSuite) TearDownSuite() {
	suite.initializer.TearDown()
}

func TestFileUpload(t *testing.T) {
	suite.Run(t, new(uploadTestSuite))
}

func (suite *uploadTestSuite) TestHTTPUpload() {
	if len(suite.cfg.HTTPServer) == 0 {
		suite.T().Skip("HTTP_SERVER variable must be set")
	}
	upload := newHTTPUpload(suite.T(), suite.cfg.HTTPServer)
	suite.testUpload(upload)
}

func (suite *uploadTestSuite) TestAzureUpload() {
	options, err := uploaders.GetAzureTestOptions(suite.T())
	require.NoError(suite.T(), err, "error getting azure test options")
	upload := newAzureUpload(suite.T(), options)
	suite.testUpload(upload)
}

func (suite *uploadTestSuite) TestAWSUpload() {
	options := uploaders.GetAWSTestOptions(suite.T())
	upload, err := newAWSUpload(suite.T(), options)
	require.NoError(suite.T(), err, "error creating AWS client")
	suite.testUpload(upload)
}

func (suite *uploadTestSuite) triggerUploads() map[string]string {
	requestPath := fmt.Sprintf("/features/%s/outbox/messages/request", featureID)
	requestedFiles := make(map[string]string)

	connMessages, err := util.NewDigitalTwinWSConnection(suite.initializer.Cfg)
	require.NoError(suite.T(), err, "Failed to create websocket connection")
	defer connMessages.Close()

	suite.startListening(connMessages, typeMessages)
	url := fmt.Sprintf("%s/inbox/messages/%s", suite.featureURL, operationTrigger)
	_, err = util.SendDigitalTwinRequest(suite.initializer.Cfg, http.MethodPost, url, map[string]interface{}{
		paramCorrelationID: "test",
	})
	require.NoError(suite.T(), err, "error sending digital twin request for trigger operation")
	err = util.ProcessWSMessages(suite.initializer.Cfg, connMessages,
		func(event *protocol.Envelope) (bool, error) {
			if requestPath == event.Path {
				request := &uploadRequest{}
				err := parseEventValue(event.Value, request)
				if err != nil {
					suite.T().Logf("cannot convert %v to upload request, will ignore the event", event.Value)
					return false, nil
				}
				if request.Options == nil {
					suite.T().Logf("no upload request options found in payload(%v)", request)
					return false, nil
				}
				if path, ok := request.Options["file.path"]; ok {
					suite.T().Logf("file upload request: %s, with correlation id: %s", path, request.CorrelationID)
					requestedFiles[request.CorrelationID] = path
					return len(requestedFiles) == uploadFilesCount, nil
				}
				suite.T().Log("file.path key not found in upload request event options")
			}
			return len(requestedFiles) == uploadFilesCount, nil
		})

	require.NoError(suite.T(), err, "error processing file upload requests")
	require.Equal(suite.T(), uploadFilesCount, len(requestedFiles), "wrong file upload request events count")
	suite.T().Logf("%v file upload requests, initiating uploads", len(requestedFiles))
	return requestedFiles
}

func (suite *uploadTestSuite) startUploads(testUpload upload, requestedFiles map[string]string, files []string) {
	connEvents, err := util.NewDigitalTwinWSConnection(suite.initializer.Cfg)
	require.NoError(suite.T(), err, "Failed to create websocket connection")
	defer connEvents.Close()

	suite.startListening(connEvents, typeEvents)

	requestedFilesRev := make(map[string]string)
	url := fmt.Sprintf("%s/inbox/messages/%s", suite.featureURL, operationStart)

	for startID, path := range requestedFiles {
		requestedFilesRev[path] = startID
		_, err := util.SendDigitalTwinRequest(suite.initializer.Cfg, http.MethodPost, url, testUpload.requestUpload(startID, path))
		require.NoError(suite.T(), err, "error sending digital twin request for trigger operation")
	}

	lastUploadPath := fmt.Sprintf("/features/%s/properties/%s", featureID, propertyLastUpload)
	var lastUploadState string

	err = util.ProcessWSMessages(suite.initializer.Cfg, connEvents,
		func(event *protocol.Envelope) (bool, error) {
			if lastUploadPath == event.Path {
				status := &uploadStatus{}
				err := parseEventValue(event.Value, status)
				if err != nil {
					suite.T().Logf("cannot convert %v to upload request, will ignore the event", event.Value)
					return false, nil
				}
				suite.T().Logf("last upload event(state: %s, progress %v)", status.State, status.Progress)
				lastUploadState = status.State
				return isTerminal(status.State), nil
			}
			return false, nil
		})
	require.NoError(suite.T(), err, "error upload status events")
	require.Equal(suite.T(), client.StateSuccess, lastUploadState, "wrong final upload state")
	for _, filePath := range files {
		startID, ok := requestedFilesRev[filePath]
		require.True(suite.T(), ok, fmt.Sprintf("no upload request event for %s", filePath))
		content, err := testUpload.download(startID)
		require.NoError(suite.T(), err, fmt.Sprintf("file %s not uploaded", filePath))
		suite.compareContent(filePath, content)
	}
}

func (suite *uploadTestSuite) testUpload(testUpload upload) {
	files, err := createTestFiles(suite.cfg.UploadDir)
	defer deleteTestFiles(files)
	require.NoError(suite.T(), err, "creating test files failed")

	requestedFiles := suite.triggerUploads()
	defer testUpload.removeUploads()

	suite.startUploads(testUpload, requestedFiles, files)
}

func (suite *uploadTestSuite) checkUploadDir() {
	files, err := os.ReadDir(suite.cfg.UploadDir)
	if err != nil {
		suite.T().Skipf("upload dir %s cannot be read - %v", suite.cfg.UploadDir, err)
	}
	if len(files) > 0 {
		suite.T().Skipf("upload dir %s must be empty", suite.cfg.UploadDir)
	}
}

func (suite *uploadTestSuite) startListening(conn *websocket.Conn, eventType string) {
	err := websocket.Message.Send(conn, fmt.Sprintf("%s?filter=like(resource:path,'/features/%s/*')", eventType, featureID))
	require.NoError(suite.T(), err, "error sending listener request")
	err = util.WaitForWSMessage(suite.initializer.Cfg, conn, fmt.Sprintf("%s:ACK", eventType))
	require.NoError(suite.T(), err, "acknowledgement not received in time")
}

func (suite *uploadTestSuite) compareContent(filePath string, received []byte) {
	expected, err := ioutil.ReadFile(filePath)
	require.NoError(suite.T(), err, fmt.Sprintf("cannot read file %s", filePath))
	require.Equal(suite.T(), string(expected), string(received), fmt.Sprintf("uploaded content of file %s differs from original", filePath))
}

func parseEventValue(props interface{}, result interface{}) error {
	jsonValue, err := json.Marshal(props)
	if err != nil {
		return err
	}
	return json.Unmarshal(jsonValue, result)
}

func createTestFiles(dir string) ([]string, error) {
	var result []string
	for i := 1; i <= uploadFilesCount; i++ {
		filePath := filepath.Join(dir, fmt.Sprintf(uploadFilesPattern, i))
		result = append(result, filePath)
		if err := writeTestContent(filePath, 10*i); err != nil {
			return result, err
		}
	}
	return result, nil
}

func deleteTestFiles(files []string) {
	for _, file := range files {
		os.Remove(file)
	}
}

func writeTestContent(filePath string, count int) error {
	data := strings.Repeat("test", count)
	return ioutil.WriteFile(filePath, []byte(data), fs.ModePerm)
}

func isTerminal(state string) bool {
	return state == client.StateSuccess || state == client.StateFailed || state == client.StateCanceled
}
