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
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/eclipse-kanto/file-upload/client"
	"github.com/eclipse-kanto/kanto/integration/util"
	"github.com/eclipse/ditto-clients-golang/protocol"
	"github.com/stretchr/testify/require"
)

type uploadRequest struct {
	CorrelationID string            `json:"correlationId"`
	Options       map[string]string `json:"options"`
}

type uploadStatus struct {
	State    string `json:"state"`
	Progress int    `json:"progress"`
}

// UploadRequests executes an operation, which triggers file upload(s) and collects the upload requests
func (suite *FileUploadSuite) UploadRequests(featureID string, operation string, params interface{}, expectedFileCount int) map[string]string {
	topicOperation := util.GetLiveMessageTopic(suite.ThingCfg.DeviceID, protocol.TopicAction(operation))
	pathOperation := util.GetFeatureInboxMessagePath(featureID, operation)
	topicRequest := util.GetLiveMessageTopic(suite.ThingCfg.DeviceID, actionRequest)
	pathRequest := util.GetFeatureOutboxMessagePath(featureID, actionRequest)

	connMessages, err := util.NewDigitalTwinWSConnection(suite.Cfg)
	require.NoError(suite.T(), err, msgFailedCreateWebsocketConnection)
	defer connMessages.Close()

	err = util.SubscribeForWSMessages(suite.Cfg, connMessages, util.StartSendMessages, fmt.Sprintf(eventFilterTemplate, featureID))
	require.NoError(suite.T(), err, "error subscribing for WS ditto messages")
	defer util.UnsubscribeFromWSMessages(suite.Cfg, connMessages, util.StopSendMessages)

	_, err = util.ExecuteOperation(suite.Cfg, suite.FeatureURL, operation, params)
	require.NoErrorf(suite.T(), err, msgErrorExecutingOperation, operation)
	requests := []interface{}{}
	var operationEvent bool
	err = util.ProcessWSMessages(suite.Cfg, connMessages,
		func(msg *protocol.Envelope) (bool, error) {
			if msg.Topic.String() == topicRequest && msg.Path == pathRequest {
				requests = append(requests, msg.Value)
				return len(requests) == expectedFileCount, nil
			} else if msg.Topic.String() == topicOperation && msg.Path == pathOperation {
				operationEvent = true
				return false, nil
			}
			return true, fmt.Errorf(msgUnexpectedValue, msg.Value)
		})
	require.NoError(suite.T(), err, "error processing file upload requests")
	require.Truef(suite.T(), operationEvent, "event for %s operation not received", operation)
	require.Equal(suite.T(), expectedFileCount, len(requests), "wrong file upload request events count")

	requestedFiles := make(map[string]string)
	for _, request := range requests {
		uploadRequest := &uploadRequest{}
		err := util.Convert(request, uploadRequest)
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

// StartUploads starts the uploads for all given file upload requests
func (suite *FileUploadSuite) StartUploads(featureID string, provider StorageProvider, requestedFiles map[string]string) {
	pathLastUpload := util.GetFeaturePropertyPath(featureID, propertyLastUpload)
	topicCreated := util.GetTwinEventTopic(suite.ThingCfg.DeviceID, protocol.ActionCreated)
	topicModified := util.GetTwinEventTopic(suite.ThingCfg.DeviceID, protocol.ActionModified)

	connEvents, err := util.NewDigitalTwinWSConnection(suite.Cfg)
	require.NoError(suite.T(), err, msgFailedCreateWebsocketConnection)
	defer connEvents.Close()

	err = util.SubscribeForWSMessages(suite.Cfg, connEvents, util.StartSendEvents, fmt.Sprintf(eventFilterTemplate, featureID))
	require.NoError(suite.T(), err, "error subscribing for WS ditto events")
	defer util.UnsubscribeFromWSMessages(suite.Cfg, connEvents, util.StopSendEvents)

	for startID, path := range requestedFiles {
		_, err := util.ExecuteOperation(suite.Cfg, suite.FeatureURL, operationStart, provider.requestUpload(startID, path))
		require.NoErrorf(suite.T(), err, msgErrorExecutingOperation, operationStart)
	}

	statuses := []interface{}{}
	err = util.ProcessWSMessages(suite.Cfg, connEvents,
		func(msg *protocol.Envelope) (bool, error) {
			if (msg.Topic.String() == topicCreated || msg.Topic.String() == topicModified) && msg.Path == pathLastUpload {
				statuses = append(statuses, msg.Value)
				return ContainsState(msg.Value, client.StateSuccess, client.StateFailed, client.StateCanceled), nil
			}
			return true, fmt.Errorf(msgUnexpectedValue, msg.Value)
		})
	require.NoError(suite.T(), err, "error processing upload status events")

	lastUploadProgress := 0
	for ind, status := range statuses {
		uploadStatus := uploadStatus{}
		err := util.Convert(status, &uploadStatus)
		require.NoErrorf(suite.T(), err, "cannot convert %v to upload status", status)
		suite.T().Logf("upload status event(%v)", uploadStatus)
		require.GreaterOrEqual(suite.T(), uploadStatus.Progress, lastUploadProgress,
			"upload status progress should be non-decreasing")
		require.LessOrEqual(suite.T(), uploadStatus.Progress, 100,
			"upload status progress should be less than or equal to 100%")
		lastUploadProgress = uploadStatus.Progress
		if ind < len(statuses)-1 {
			require.Equal(suite.T(), client.StateUploading, uploadStatus.State, "wrong transitional upload state")
		} else {
			require.Equal(suite.T(), client.StateSuccess, uploadStatus.State, "wrong final upload state")
		}
	}
}

// ContainsState checks if a status "state" property is contained in the specified states
func ContainsState(status interface{}, states ...string) bool {
	if props, ok := status.(map[string]interface{}); ok {
		actualState := props["state"]
		for _, state := range states {
			if actualState == state {
				return true
			}
		}
	}
	return false
}

// DownloadURL retrieves the download url for a given correlation id
func GetDownloadURL(provider StorageProvider, correlationID string) (string, error) {
	return provider.downloadURL(correlationID)
}

// File/Directory test util functionalities start here

// AssertEmptyDir checks if a directory is empty
func (suite *FileUploadSuite) AssertEmptyDir(dir string) {
	files, err := os.ReadDir(dir)
	if err != nil {
		suite.T().Fatalf("directory %s cannot be read - %v", dir, err)
	}
	if len(files) > 0 {
		suite.T().Fatalf("directory %s must be empty", dir)
	}
}

// CreateTestFiles creates a given number of files in a given directory, filling them with some test bytes
func CreateTestFiles(dir string, fileCount int) ([]string, error) {
	var result []string
	for i := 1; i <= fileCount; i++ {
		filePath := filepath.Join(dir, fmt.Sprintf(uploadFilesPattern, i))
		result = append(result, filePath)
		if err := writeTestContent(filePath, 10*i); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func writeTestContent(filePath string, count int) error {
	data := strings.Repeat("test", count)
	return os.WriteFile(filePath, []byte(data), fs.ModePerm)
}

// RemoveFilesSilently removes all files from a given directory
func (suite *FileUploadSuite) RemoveFilesSilently(dir string) {
	files, err := os.ReadDir(dir)
	if err != nil {
		suite.T().Logf("error reading files from directory %s(%v)", dir, err)
		return
	}
	for _, file := range files {
		path := filepath.Join(dir, file.Name())
		if err := os.Remove(path); err != nil {
			suite.T().Logf("error removing file %s(%v)", path, err)
		}
	}
}

// AssertContent asserts that the content of a file matches with the actual bytes
func (suite *FileUploadSuite) AssertContent(filePath string, actual []byte) {
	expected, err := os.ReadFile(filePath)
	require.NoErrorf(suite.T(), err, "cannot read file %s", filePath)
	require.Equalf(suite.T(), string(expected), string(actual), "actual content of file %s differs from original", filePath)
}
