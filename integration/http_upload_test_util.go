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
	"io/ioutil"
	"net/http"
	"path/filepath"
	"testing"
)

const (
	httpsMethod = "https.method"
	httpsURL    = "https.url"
)

type httpUpload struct {
	url     string
	uploads map[string]string
	t       *testing.T
}

func newHTTPUpload(t *testing.T, url string) *httpUpload {
	return &httpUpload{
		url:     url,
		uploads: make(map[string]string),
		t:       t,
	}
}

func (upload *httpUpload) requestUpload(correlationID string, filePath string) map[string]interface{} {
	file := filepath.Base(filePath)
	url := fmt.Sprintf("%s/%s", upload.url, file)
	upload.uploads[correlationID] = url
	return map[string]interface{}{
		paramCorrelationID: correlationID,
		paramOptions: map[string]string{
			httpsMethod: http.MethodPut,
			httpsURL:    url,
			fmt.Sprintf("https.header.%s", paramCorrelationID): correlationID,
		},
	}
}

func (upload *httpUpload) download(correlationID string) ([]byte, error) {
	url, ok := upload.uploads[correlationID]
	if !ok {
		return nil, fmt.Errorf("no upload with correlation id: %s", correlationID)
	}
	response, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	return ioutil.ReadAll(response.Body)
}

func (upload *httpUpload) removeUploads() {
	client := &http.Client{}
	for _, url := range upload.uploads {
		req, err := http.NewRequest(http.MethodDelete, url, nil)
		if err != nil {
			upload.t.Logf("error creating delete request to %s - %v", url, err)
			continue
		}
		resp, err := client.Do(req)
		if resp != nil {
			upload.t.Logf("delete response code %d from %s", resp.StatusCode, url)
			defer resp.Body.Close()
		} else {
			upload.t.Logf("error sending delete request to %s - %v", url, err)
		}
	}
}
