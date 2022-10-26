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

package integration

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eclipse-kanto/file-upload/client"
	"github.com/eclipse/ditto-clients-golang"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

var uploadDir string

func (suite *testSuite) SetupSuite() {
	cfg := &testConfig{}

	suite.T().Log(getConfigHelp(*cfg))

	if err := initConfigFromEnv(cfg); err != nil {
		suite.T().Skip(err)
	}

	suite.T().Logf("test config: %+v", *cfg)

	opts := mqtt.NewClientOptions().
		AddBroker(cfg.Broker).
		SetClientID(uuid.New().String()).
		SetKeepAlive(30 * time.Second).
		SetCleanSession(true).
		SetAutoReconnect(true)

	mqttClient := mqtt.NewClient(opts)

	if token := mqttClient.Connect(); token.Wait() && token.Error() != nil {
		require.NoError(suite.T(), token.Error(), "connect to MQTT broker")
	}

	thingCfg, err := getThingConfig(mqttClient)
	if err != nil {
		mqttClient.Disconnect(uint(cfg.MqttQuiesceMs))
		require.NoError(suite.T(), err, "get thing config")
	}

	suite.T().Logf("thing config: %+v", *thingCfg)

	dittoClient, err := ditto.NewClientMqtt(mqttClient, ditto.NewConfiguration())
	if err == nil {
		err = dittoClient.Connect()
	}

	if err != nil {
		mqttClient.Disconnect(uint(cfg.MqttQuiesceMs))
		require.NoError(suite.T(), err, "initialize ditto client")
	}

	suite.dittoClient = dittoClient
	suite.mqttClient = mqttClient
	suite.cfg = cfg
	suite.thingCfg = thingCfg

	suite.thingURL = fmt.Sprintf("%s/api/2/things/%s", strings.TrimSuffix(cfg.DittoAddress, "/"), thingCfg.DeviceID)
	suite.featureURL = fmt.Sprintf("%s/features/%s", suite.thingURL, featureID)

	uploadDir, err = setupUploadDir()
	require.Nil(suite.T(), err, "get configured file upload directory")
	require.NotEmpty(suite.T(), uploadDir, "get configured file upload directory")
	suite.T().Logf("upload dir - %s", uploadDir)
}

func (suite *testSuite) TearDownSuite() {
	suite.dittoClient.Disconnect()
	suite.mqttClient.Disconnect(uint(suite.cfg.MqttQuiesceMs))
}

func setupUploadDir() (string, error) {
	var uploadDir string
	data, err := ioutil.ReadFile(configFile)
	if err != nil {
		return uploadDir, err
	}
	config := make(map[string]interface{})
	err = json.Unmarshal(data, &config)
	if err != nil {
		return uploadDir, err
	}
	if files, ok := config["files"]; ok {
		uploadDir = filepath.Dir(files.(string))
	}
	return uploadDir, nil
}

func TestFileUpload(t *testing.T) {
	suite.Run(t, new(testSuite))
}

func (suite *testSuite) TestUploadHTTP() {
	suite.T().Log("test file upload over HTTP")
	uploadHandler := newHTTPUploadHandler()
	suite.testUpload(uploadHandler)
}

func (suite *testSuite) testUpload(uploadHandler uploadHandler) {
	files, err := createTestFiles(uploadDir)
	defer deleteTestFiles(files)
	require.Nil(suite.T(), err, "create test files")

	err = uploadHandler.prepare()
	require.Nil(suite.T(), err, "prepare upload handler")
	defer uploadHandler.dispose()

	type uploadRequest struct {
		CorrelationID string            `json:"correlationId"`
		Options       map[string]string `json:"options"`
	}
	requestPath := fmt.Sprintf("/features/%s/outbox/messages/request", featureID)
	filePaths := make(map[string]string)
	finishedGettingFilePaths := false
	conn, _ := suite.startEventListener(typeMessages, func(props map[string]interface{}) bool {
		if finishedGettingFilePaths {
			return true
		}
		if requestPath == props["path"] {
			if value, ok := props["value"]; ok {
				jsonValue, err := json.Marshal(value)
				if err != nil {
					return false // skip
				}
				uploadRequest := &uploadRequest{}
				if err = json.Unmarshal([]byte(jsonValue), uploadRequest); err != nil {
					return false
				}
				if path, ok := uploadRequest.Options["file.path"]; ok {
					suite.T().Logf("file upload request: %s, with correlation id: %s", path, uploadRequest.CorrelationID)
					filePaths[uploadRequest.CorrelationID] = path
					return false
				}
			}
		}
		return false
	})
	defer conn.Close()
	correlationID := "test"
	suite.trigger(correlationID)
	time.Sleep(uploadRequestTimeout * time.Second)
	finishedGettingFilePaths = true
	suite.T().Logf("%v file upload requests, initiating uploads", len(filePaths))

	path := fmt.Sprintf("/features/%s/properties/lastUpload", featureID)
	var lastUploadState string
	_, chEvent := suite.startEventListener(typeEvents, func(props map[string]interface{}) bool {
		if path == props["path"] {
			if value, ok := props["value"]; ok {
				lastUpload, check := value.(map[string]interface{})
				lastUploadState = lastUpload["state"].(string)
				suite.T().Logf("last upload event(state: %s, progress %v)", lastUploadState, lastUpload["progress"])
				return check && isTerminal(lastUploadState)
			}
		}
		return false
	})
	filePathsRev := make(map[string]string)
	for startID, path := range filePaths {
		filePathsRev[path] = startID
		suite.execCommand(operationStart, uploadHandler.getStartOptions(startID, path))
	}
	require.True(suite.T(), suite.awaitChan(chEvent), "upload finished event not received")
	require.Equal(suite.T(), client.StateSuccess, lastUploadState, "wrong final upload state")
	for _, filePath := range files {
		startID, ok := filePathsRev[filePath]
		require.True(suite.T(), ok, "upload request events")
		content, err := uploadHandler.getContent(startID)
		require.Nil(suite.T(), err, "uploaded files")
		suite.compareContent(filePath, content)
	}
}

func (suite *testSuite) trigger(correlationID string) {
	params := map[string]interface{}{
		correlationID: correlationID,
	}
	suite.execCommand(operationTrigger, params)
}

func (suite *testSuite) compareContent(filePath string, received []byte) {
	expected, err := ioutil.ReadFile(filePath)
	require.Nil(suite.T(), err, "uploaded files")
	require.Equal(suite.T(), expected, received, "uploaded files")
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
