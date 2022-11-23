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
	"time"

	"github.com/eclipse-kanto/kanto/integration/util"
	"github.com/stretchr/testify/suite"
)

type fileUploadSuite struct {
	suite.Suite
	util.SuiteInitializer

	thingURL   string
	featureURL string
	uploadCfg  uploadTestConfig
}

type httpFileUploadSuite struct {
	fileUploadSuite
}

type azureFileUploadSuite struct {
	fileUploadSuite
}

type awsFileUploadSuite struct {
	fileUploadSuite
}

type uploadTestConfig struct {
	UploadDir            string        `env:"FUT_UPLOAD_DIR"`
	HTTPServer           string        `env:"FUT_HTTP_SERVER"`
	UploadFilesTimeout   time.Duration `env:"FUT_UPLOAD_FILES_TIMEOUT" envDefault:"20s"`
	UploadRequestTimeout time.Duration `env:"FUT_UPLOAD_REQUEST_TIMEOUT" envDefault:"10s"`
}

type upload interface {
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

	eventFilterTemplate = "like(resource:path,'%s')"

	typeEvents   = "START-SEND-EVENTS"
	typeMessages = "START-SEND-MESSAGES"

	msgNoUploadCorrelationID           = "no upload with correlation id: %s"
	msgErrorExecutingOperation         = "error executing operation %s"
	msgUnexpectedValue                 = "unexpected value: %v"
	msgFailedCreateWebsocketConnection = "failed to create websocket connection"
)
