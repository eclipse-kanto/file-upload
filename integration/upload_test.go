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
)

type uploadRequest struct {
	CorrelationID string            `json:"correlationId"`
	Options       map[string]string `json:"options"`
}

type uploadStatus struct {
	State    string `json:"state"`
	Progress int    `json:"progress"`
}

func (suite *fileUploadSuite) SetupSuite() {
	suite.Setup(suite.T())

	opts := env.Options{RequiredIfNoDef: false}
	suite.uploadCfg = uploadTestConfig{}
	require.NoError(suite.T(), env.Parse(&suite.uploadCfg, opts), "failed to process upload environment variables")
	suite.T().Logf("upload test configuration - %v", suite.uploadCfg)
	suite.checkUploadDir()

	suite.thingURL = util.GetThingURL(suite.Cfg.DigitalTwinAPIAddress, suite.ThingCfg.DeviceID)
	suite.featureURL = util.GetFeatureURL(suite.thingURL, featureID)
}

func (suite *fileUploadSuite) TearDownSuite() {
	suite.TearDown()
}

func TestHTTPFileUpload(t *testing.T) {
	suite.Run(t, new(httpFileUploadSuite))
}

func TestAzureFileUpload(t *testing.T) {
	suite.Run(t, new(azureFileUploadSuite))
}

func TestAWSFileUpload(t *testing.T) {
	suite.Run(t, new(awsFileUploadSuite))
}

func (suite *httpFileUploadSuite) TestFileUpload() {
	if len(suite.uploadCfg.HTTPServer) == 0 {
		suite.T().Fatal("http server must be set")
	}
	upload := newHTTPUpload(suite.T(), suite.uploadCfg.HTTPServer)
	suite.testUpload(upload)
}

func (suite *azureFileUploadSuite) TestFileUpload() {
	creds, err := uploaders.GetAzureTestCredentials()
	require.NoError(suite.T(), err, "please set azure environment variables")
	options, err := uploaders.GetAzureTestOptions(creds)
	require.NoError(suite.T(), err, "error getting azure test options")
	upload := newAzureUpload(suite.T(), options)
	suite.testUpload(upload)
}

func (suite *awsFileUploadSuite) TestFileUpload() {
	creds, err := uploaders.GetAWSTestCredentials()
	require.NoError(suite.T(), err, "please set aws environment variables")
	options := uploaders.GetAWSTestOptions(creds)
	upload, err := newAWSUpload(suite.T(), options)
	require.NoError(suite.T(), err, "error creating AWS client")
	suite.testUpload(upload)
}

func (suite *fileUploadSuite) triggerUploads() map[string]string {
	topicTrigger := util.GetLiveMessageTopic(suite.ThingCfg.DeviceID, operationTrigger)
	pathTrigger := getFeatureInboxMessagePath(featureID, operationTrigger)
	topicRequest := util.GetLiveMessageTopic(suite.ThingCfg.DeviceID, actionRequest)
	pathRequest := util.GetFeatureOutboxMessagePath(featureID, actionRequest)

	connMessages, err := util.NewDigitalTwinWSConnection(suite.Cfg)
	require.NoError(suite.T(), err, msgFailedCreateWebsocketConnection)
	defer connMessages.Close()

	util.SubscribeForWSMessages(suite.Cfg, connMessages, typeMessages, fmt.Sprintf(eventFilterTemplate, featureID))
	_, err = util.ExecuteOperation(suite.Cfg, suite.featureURL, operationTrigger, nil)
	require.NoErrorf(suite.T(), err, msgErrorExecutingOperation, operationTrigger)
	requests := []interface{}{}
	var triggerEvent bool
	err = util.ProcessWSMessages(suite.Cfg, connMessages,
		func(msg *protocol.Envelope) (bool, error) {
			if msg.Topic.String() == topicRequest && msg.Path == pathRequest {
				requests = append(requests, msg.Value)
				return len(requests) == uploadFilesCount, nil
			} else if msg.Topic.String() == topicTrigger && msg.Path == pathTrigger {
				triggerEvent = true
				return false, nil
			}
			return true, fmt.Errorf(msgUnexpectedValue, msg.Value)
		})
	require.NoError(suite.T(), err, "error processing file upload requests")
	require.True(suite.T(), triggerEvent, "event for trigger operation not received")
	require.Equal(suite.T(), uploadFilesCount, len(requests), "wrong file upload request events count")

	requestedFiles := make(map[string]string)
	for _, request := range requests {
		uploadRequest := &uploadRequest{}
		err := parseEventValue(request, uploadRequest)
		require.NoErrorf(suite.T(), err, "cannot convert %v to upload request", request)
		require.NotNilf(suite.T(), uploadRequest.Options, "no upload request options found in payload(%v)", uploadRequest)
		path, ok := uploadRequest.Options[keyFilePath]
		require.Truef(suite.T(), ok, "%s key not found in upload request event options", keyFilePath)
		suite.T().Logf("file upload request: %s, with correlation id: %s", path, uploadRequest.CorrelationID)
		requestedFiles[uploadRequest.CorrelationID] = path
	}
	suite.T().Logf("%v file upload requests, initiating uploads", len(requestedFiles))
	return requestedFiles
}

func (suite *fileUploadSuite) startUploads(testUpload upload, requestedFiles map[string]string, files []string) {
	pathLastUpload := util.GetFeaturePropertyPath(featureID, propertyLastUpload)
	topicModified := util.GetTwinEventTopic(suite.ThingCfg.DeviceID, protocol.ActionModified)

	connEvents, err := util.NewDigitalTwinWSConnection(suite.Cfg)
	require.NoError(suite.T(), err, msgFailedCreateWebsocketConnection)
	defer connEvents.Close()

	util.SubscribeForWSMessages(suite.Cfg, connEvents, typeEvents, fmt.Sprintf(eventFilterTemplate, featureID))
	fileIDs := make(map[string]string)
	for startID, path := range requestedFiles {
		fileIDs[path] = startID
		_, err := util.ExecuteOperation(suite.Cfg, suite.featureURL, operationStart, testUpload.requestUpload(startID, path))
		require.NoErrorf(suite.T(), err, msgErrorExecutingOperation, operationStart)
	}

	statuses := []interface{}{}
	err = util.ProcessWSMessages(suite.Cfg, connEvents,
		func(msg *protocol.Envelope) (bool, error) {
			if msg.Topic.String() == topicModified && msg.Path == pathLastUpload {
				statuses = append(statuses, msg.Value)
				return isTerminal(msg.Value), nil
			}
			return true, fmt.Errorf(msgUnexpectedValue, msg.Value)
		})
	require.NoError(suite.T(), err, "error processing upload status events")

	lastUploadProgress := 0
	for ind, status := range statuses {
		uploadStatus := uploadStatus{}
		err := parseEventValue(status, &uploadStatus)
		require.NoErrorf(suite.T(), err, "cannot convert %v to upload status", status)
		suite.T().Logf("upload status event(%v)", uploadStatus)
		require.GreaterOrEqual(suite.T(), uploadStatus.Progress, lastUploadProgress,
			"upload status progress should be non-decreasing")
		require.LessOrEqual(suite.T(), uploadStatus.Progress, 100, "upload status progress should be less than 100%")
		lastUploadProgress = uploadStatus.Progress
		if ind < len(statuses)-1 {
			require.Equal(suite.T(), client.StateUploading, uploadStatus.State, "wrong transitional upload state")
		} else {
			require.Equal(suite.T(), client.StateSuccess, uploadStatus.State, "wrong final upload state")
		}
	}
	for _, filePath := range files {
		startID, ok := fileIDs[filePath]
		require.True(suite.T(), ok, fmt.Sprintf("no upload request event for %s", filePath))
		content, err := testUpload.download(startID)
		require.NoError(suite.T(), err, fmt.Sprintf("file %s not uploaded", filePath))
		suite.compareContent(filePath, content)
	}
}

func (suite *fileUploadSuite) testUpload(testUpload upload) {
	files, err := createTestFiles(suite.uploadCfg.UploadDir)
	defer suite.deleteTestFiles(files)
	require.NoError(suite.T(), err, "creating test files failed")

	requestedFiles := suite.triggerUploads()
	defer testUpload.removeUploads()

	suite.startUploads(testUpload, requestedFiles, files)
}

func (suite *fileUploadSuite) checkUploadDir() {
	files, err := os.ReadDir(suite.uploadCfg.UploadDir)
	if err != nil {
		suite.T().Fatalf("upload dir %s cannot be read - %v", suite.uploadCfg.UploadDir, err)
	}
	if len(files) > 0 {
		suite.T().Fatalf("upload dir %s must be empty", suite.uploadCfg.UploadDir)
	}
}

func (suite *fileUploadSuite) compareContent(filePath string, received []byte) {
	expected, err := os.ReadFile(filePath)
	require.NoError(suite.T(), err, fmt.Sprintf("cannot read file %s", filePath))
	require.Equal(suite.T(), string(expected), string(received), fmt.Sprintf("uploaded content of file %s differs from original", filePath))
}

func (suite *fileUploadSuite) deleteTestFiles(files []string) {
	for _, file := range files {
		if err := os.Remove(file); err != nil {
			suite.T().Logf("error deleting test file %s(%v)", file, err)
		}
	}
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

func writeTestContent(filePath string, count int) error {
	data := strings.Repeat("test", count)
	return os.WriteFile(filePath, []byte(data), fs.ModePerm)
}

func isTerminal(status interface{}) bool {
	if props, ok := status.(map[string]interface{}); ok {
		state := props["state"]
		return state == client.StateSuccess || state == client.StateFailed || state == client.StateCanceled
	}
	return false
}

func getFeatureInboxMessagePath(featureID string, name string) string {
	return fmt.Sprintf(featureInboxMessagePathTemplate, featureID, name)
}
