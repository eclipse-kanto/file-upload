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
	"github.com/eclipse-kanto/kanto/integration/util"
	"github.com/stretchr/testify/suite"
)

// FileUploadSuite tests the file upload functionalities, using a provided configuration
type FileUploadSuite struct {
	suite.Suite
	util.SuiteInitializer

	ThingURL   string
	FeatureURL string

	uploadCfg uploadTestConfig
	provider  storageProvider
}

type httpFileUploadSuite struct {
	FileUploadSuite
}

type azureFileUploadSuite struct {
	FileUploadSuite
}

type awsFileUploadSuite struct {
	FileUploadSuite
}

// uploadTestConfig contains the test configuration data, such as upload directory and http server URL
type uploadTestConfig struct {
	UploadDir  string `env:"FUT_UPLOAD_DIR"`
	HTTPServer string `env:"FUT_HTTP_SERVER"`
}

// storageProvider contains file upload, download and remove logic for different storage providers(i.e Azure, AWS, generic)
type storageProvider interface {
	requestUpload(correlationID string, filePath string) map[string]interface{}
	downloadURL(correlationID string) (string, error)
	download(correlationID string) ([]byte, error)
	removeUploads()
}

// Provider represents a storage provider type
type Provider int

// Constants for different storage providers
const (
	AzureStorageProvider Provider = iota
	AWSStorageProvider
	GenericStorageProvider
)

const (
	featureID = "AutoUploadable"

	uploadFilesPattern = "upload_it_%d.txt"
	uploadFilesCount   = 5

	paramCorrelationID = "correlationID"
	paramOptions       = "options"
	operationTrigger   = "trigger"
	operationStart     = "start"
	propertyLastUpload = "lastUpload"
	keyFilePath        = "file.path"

	actionRequest = "request"

	eventFilterTemplate = "like(resource:path,'/features/%s/*')"

	msgNoUploadCorrelationID           = "no upload with correlation id: %s"
	msgErrorExecutingOperation         = "error executing operation %s"
	msgUnexpectedValue                 = "unexpected value: %v"
	msgFailedCreateWebsocketConnection = "failed to create websocket connection"
)
