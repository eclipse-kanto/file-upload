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
	"io"
	"net/http"
	"path/filepath"
	"testing"
)

const (
	paramHTTPSMethod = "https.method"
	paramHTTPSURL    = "https.url"
)

// httpStorageProvider provides testing functionalities for a generic HTTP storage provider
type httpStorageProvider struct {
	location string
	uploads  map[string]string
	t        *testing.T
}

// NewHTTPStorageProvider creates an implementation of the StorageProvider interface for a generic HTTP storage provider
func NewHTTPStorageProvider(t *testing.T, url string) StorageProvider {
	return httpStorageProvider{
		location: fmt.Sprintf("%s/%%s", url),
		uploads:  make(map[string]string),
		t:        t,
	}
}

func (provider httpStorageProvider) requestUpload(correlationID string, filePath string) map[string]interface{} {
	file := filepath.Base(filePath)
	url := fmt.Sprintf(provider.location, file)
	provider.uploads[correlationID] = url
	return map[string]interface{}{
		paramCorrelationID: correlationID,
		paramOptions: map[string]string{
			paramHTTPSMethod: http.MethodPut,
			paramHTTPSURL:    url,
		},
	}
}

func (provider httpStorageProvider) download(correlationID string) ([]byte, error) {
	url, err := provider.downloadURL(correlationID)
	if err != nil {
		return nil, err
	}
	response, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	return io.ReadAll(response.Body)
}

func (provider httpStorageProvider) downloadURL(correlationID string) (string, error) {
	url, ok := provider.uploads[correlationID]
	if !ok {
		return "", fmt.Errorf(msgNoUploadCorrelationID, correlationID)
	}
	return url, nil
}

func (provider httpStorageProvider) removeUploads() {
	client := &http.Client{}
	for _, url := range provider.uploads {
		req, err := http.NewRequest(http.MethodDelete, url, nil)
		if err != nil {
			provider.t.Logf("error creating delete request to %s - %v", url, err)
			continue
		}
		resp, err := client.Do(req)
		if err != nil {
			provider.t.Logf("error sending delete request to %s - %v", url, err)
		} else {
			provider.t.Logf("delete response code %d from %s", resp.StatusCode, url)
			defer resp.Body.Close()
		}
	}
}
