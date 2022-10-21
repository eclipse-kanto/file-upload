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
	"net"
	"net/http"
	"time"
)

const (
	port         = 9001
	https_method = "https.method"
	https_url    = "https.url"
)

type serveHandler struct {
	uploadedData map[string]interface{}
}

type httpUploadHandler struct {
	server *http.Server
}

func newHTTPUploadHandler() *httpUploadHandler {
	return &httpUploadHandler{
		server: &http.Server{
			Addr: fmt.Sprintf("localhost:%d", port),
			Handler: &serveHandler{
				uploadedData: make(map[string]interface{}),
			},
		},
	}
}

func (h *serveHandler) ServeHTTP(resp http.ResponseWriter, request *http.Request) {
	key := request.Header.Get(paramCorrelationID)
	buff, err := ioutil.ReadAll(request.Body)
	if err != nil {
		h.uploadedData[key] = err
	} else {
		h.uploadedData[key] = buff
	}
}

func (uploadHandler *httpUploadHandler) prepare() error {
	ln, err := net.Listen("tcp", uploadHandler.server.Addr)
	if err != nil {
		return err
	}

	go func() {
		uploadHandler.server.Serve(ln)
	}()
	time.Sleep(time.Second)
	return nil
}

func (uploadHandler *httpUploadHandler) getStartOptions(correlationID string, filePath string) map[string]interface{} {
	return map[string]interface{}{
		paramCorrelationID: correlationID,
		"options": map[string]string{
			https_method: http.MethodPost,
			https_url:    fmt.Sprintf("http://%s/uploads/%s", uploadHandler.server.Addr, filePath),
			fmt.Sprintf("https.header.%s", paramCorrelationID): correlationID,
		},
	}
}

func (uploadHandler *httpUploadHandler) getContent(correlationID string) ([]byte, error) {
	serveHandler := uploadHandler.server.Handler.(*serveHandler)
	result := serveHandler.uploadedData[correlationID]
	if value, ok := result.([]byte); ok {
		return value, nil
	}
	if err, ok := result.(error); ok {
		return nil, err
	}
	return nil, nil
}

func (uploadHandler *httpUploadHandler) dispose() {
	if uploadHandler.server != nil {
		uploadHandler.server.Close()
	}
}
