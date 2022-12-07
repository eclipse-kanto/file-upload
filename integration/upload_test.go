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
	"fmt"
	"testing"

	"github.com/caarlos0/env/v6"
	"github.com/eclipse-kanto/kanto/integration/util"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

func (suite *FileUploadSuite) SetupSuite() {
	suite.Setup(suite.T())

	opts := env.Options{RequiredIfNoDef: false}
	suite.uploadCfg = UploadTestConfig{}
	require.NoError(suite.T(), env.Parse(&suite.uploadCfg, opts), "failed to process upload environment variables")
	suite.T().Logf("upload test configuration - %v", suite.uploadCfg)
	suite.AssertEmptyDir(suite.uploadCfg.UploadDir)

	suite.ThingURL = util.GetThingURL(suite.Cfg.DigitalTwinAPIAddress, suite.ThingCfg.DeviceID)
	suite.FeatureURL = util.GetFeatureURL(suite.ThingURL, featureID)
}

func (suite *FileUploadSuite) TearDownSuite() {
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
	provider := NewHTTPStorageProvider(suite.T(), suite.uploadCfg.HTTPServer)
	suite.testUpload(provider)
}

func (suite *azureFileUploadSuite) TestFileUpload() {
	provider := NewAzureStorageProvider(suite.T())
	suite.testUpload(provider)
}

func (suite *awsFileUploadSuite) TestFileUpload() {
	provider := NewAWSStorageProvider(suite.T())
	suite.testUpload(provider)
}

func (suite *FileUploadSuite) checkUploadedFiles(upload StorageProvider, requestedFiles map[string]string, files []string) {
	fileIDs := make(map[string]string)
	for startID, path := range requestedFiles {
		fileIDs[path] = startID
	}

	for _, filePath := range files {
		startID, ok := fileIDs[filePath]
		require.True(suite.T(), ok, fmt.Sprintf("no upload request event for %s", filePath))
		content, err := upload.download(startID)
		require.NoError(suite.T(), err, fmt.Sprintf("file %s not uploaded", filePath))
		suite.AssertContent(filePath, content)
	}
}

func (suite *FileUploadSuite) testUpload(provider StorageProvider) {
	files, err := CreateTestFiles(suite.uploadCfg.UploadDir, uploadFilesCount)
	defer suite.RemoveFilesSilently(suite.uploadCfg.UploadDir)
	require.NoError(suite.T(), err, "creating test files failed")

	requestedFiles := suite.UploadRequests(featureID, operationTrigger, nil, uploadFilesCount)
	defer provider.removeUploads()

	suite.StartUploads(featureID, provider, requestedFiles)
	suite.checkUploadedFiles(provider, requestedFiles, files)
}
