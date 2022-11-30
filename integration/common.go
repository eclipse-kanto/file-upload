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

// FileUploadSuite is the main testing structure for this integration test
type FileUploadSuite struct {
	suite.Suite
	util.SuiteInitializer

	ThingURL   string
	FeatureURL string

	uploadCfg UploadTestConfig
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

// UploadTestConfig contains the configuration data for this integration test
type UploadTestConfig struct {
	UploadDir  string `env:"FUT_UPLOAD_DIR"`
	HTTPServer string `env:"FUT_HTTP_SERVER"`
}

// Upload is the base structure for testing different storage providers(i.e azure, aws, generic)
type Upload interface {
	GetDownloadURL(correlationID string) (string, error)

	requestUpload(correlationID string, filePath string) map[string]interface{}
	download(correlationID string) ([]byte, error)
	removeUploads()
}

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

	typeEvents   = "START-SEND-EVENTS"
	typeMessages = "START-SEND-MESSAGES"

	msgNoUploadCorrelationID           = "no upload with correlation id: %s"
	msgErrorExecutingOperation         = "error executing operation %s"
	msgUnexpectedValue                 = "unexpected value: %v"
	msgFailedCreateWebsocketConnection = "failed to create websocket connection"
)
