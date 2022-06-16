// Copyright (c) 2021 Contributors to the Eclipse Foundation
//
// See the NOTICE file(s) distributed with this work for additional
// information regarding copyright ownership.
//
// This program and the accompanying materials are made available under the
// terms of the Eclipse Public License 2.0 which is available at
// http://www.eclipse.org/legal/epl-2.0
//
// SPDX-License-Identifier: EPL-2.0

package client

import (
	"errors"
	"net/http"
	"path/filepath"

	"github.com/eclipse-kanto/file-upload/logger"
	MQTT "github.com/eclipse/paho.mqtt.golang"
)

// FileUpload uses the AutoUploadable feature to implement generic file upload.
// AutoUploadable ss performing all communication with the backend, FileUpload only specifies the files to be uploaded.
type FileUpload struct {
	filesGlob string

	uploadable *AutoUploadable
}

// NewFileUpload construct FileUpload from the provided configurations
func NewFileUpload(filesGlob string, mqttClient MQTT.Client, edgeCfg *EdgeConfiguration, uploadableCfg *UploadableConfig) (*FileUpload, error) {
	result := &FileUpload{}

	result.filesGlob = filesGlob

	uploadable, err := NewAutoUploadable(mqttClient, edgeCfg, uploadableCfg, result,
		"com.bosch.iot.suite.manager.upload:AutoUploadable:1.0.0", "com.bosch.iot.suite.manager.upload:Uploadable:1.0.0")

	if err != nil {
		return nil, err
	}

	result.uploadable = uploadable

	return result, nil
}

// Connect connects the FileUpload feature to the Ditto endpoint
func (fu *FileUpload) Connect() error {
	return fu.uploadable.Connect()
}

// Disconnect disconnects the FileUpload feature to the Ditto endpoint
func (fu *FileUpload) Disconnect() {
	fu.uploadable.Disconnect()
}

// DoTrigger triggers file upload operation.
// Can be invoked from the backend or from periodic upload tick
func (fu *FileUpload) DoTrigger(correlationID string, options map[string]string) error {
	single := fu.uploadable.cfg.SingleUpload
	if options["force"] == "true" {
		single = false
	}

	if single && fu.uploadable.uploads.hasPendingUploads() {
		return errors.New("there is an ongoing upload -  set the 'force' option to 'true' to force trigger the upload")
	}

	files, err := filepath.Glob(fu.filesGlob)
	if err != nil {
		logger.Errorf("failed to trigger upload %s: %v", correlationID, err)

		return err
	}

	fu.uploadable.UploadFiles(correlationID, files, options)

	return nil
}

// HandleOperation is invoked from the base AutoUploadable feature to handle unknown operations.
// FileUpload returns error, because it does not add any new operations to the AutoUploadable feature
func (fu *FileUpload) HandleOperation(operation string, payload []byte) *ErrorResponse {
	return &ErrorResponse{http.StatusBadRequest, "Unsupported operation: " + operation}
}

// OnTick triggers periodic file uploads. Invoked from the periodic executor in AutoUploadable
func (fu *FileUpload) OnTick() {
	err := fu.DoTrigger(fu.uploadable.nextgUID(), nil)

	if err != nil {
		logger.Errorf("error on periodic trigger: %v", err)
	}
}
