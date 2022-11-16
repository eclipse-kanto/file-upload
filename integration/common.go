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

type uploadTestSuite struct {
	suite.Suite

	initializer util.SuiteInitializer
	thingURL    string
	featureURL  string
	cfg         uploadTestConfig
}

type uploadTestConfig struct {
	UploadDir  string `env:"UPLOAD_DIR"`
	HTTPServer string `env:"HTTP_SERVER"`
}

type upload interface {
	requestUpload(correlationID string, filePath string) map[string]interface{}
	download(correlationID string) ([]byte, error)
	removeUploads()
}

const (
	featureID = "AutoUploadable"

	uploadFilesTimeout   = 20
	uploadRequestTimeout = 10
	uploadFilesPattern   = "upload_it_%d.txt"
	uploadFilesCount     = 5

	paramCorrelationID = "correlationID"
	paramOptions       = "options"
	operationTrigger   = "trigger"
	operationStart     = "start"
	propertyLastUpload = "lastUpload"

	typeEvents   = "START-SEND-EVENTS"
	typeMessages = "START-SEND-MESSAGES"
)
