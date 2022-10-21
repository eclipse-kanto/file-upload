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
	"github.com/eclipse/ditto-clients-golang"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/stretchr/testify/suite"
)

type uploadSuite struct {
	suite.Suite

	mqttClient  mqtt.Client
	dittoClient *ditto.Client

	cfg      *testConfig
	thingCfg *thingConfig

	thingURL   string
	featureURL string
}

type testConfig struct {
	Broker                   string `def:"tcp://localhost:1883"`
	MqttQuiesceMs            int    `def:"500"`
	MqttAcknowledgeTimeoutMs int    `def:"3000"`

	DittoAddress string

	DittoUser     string `def:"ditto"`
	DittoPassword string `def:"ditto"`

	EventTimeoutMs  int `def:"30000"`
	StatusTimeoutMs int `def:"10000"`

	TimeDeltaMs int `def:"5000"`
}

type thingConfig struct {
	DeviceID string `json:"deviceId"`
	TenantID string `json:"tenantId"`
	PolicyID string `json:"policyId"`
}

type uploadHandler interface {
	prepare() error
	getStartOptions(correlationID string, filePath string) map[string]interface{}
	getContent(correlationID string) ([]byte, error)
	dispose()
}

const (
	envVariablesPrefix = "SCT"
	featureID          = "AutoUploadable"

	uploadFilesTimeout   = 20
	uploadRequestTimeout = 10
	uploadFilesPattern   = "upload_it_%d.txt"
	uploadFilesCount     = 5

	configFile         = "/etc/file-upload/config.json"
	paramCorrelationID = "correlationID"
	operationTrigger   = "trigger"
	operationStart     = "start"
	propertyLastUpload = "lastUpload"
)

type lastUpload struct {
	Progress      int    `json:"progress"`
	CorrelationID string `json:"correlationId"`
	StartTime     string `json:"startTime"`
	State         string `json:"state"`
	EndTime       string `json:"endTime"`
	Message       string `json:"message"`
	StatusCode    string `json:"statusCode"`
}
