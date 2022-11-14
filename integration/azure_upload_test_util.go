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
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/eclipse-kanto/file-upload/client"
	"github.com/eclipse-kanto/file-upload/uploaders"
)

type azureUpload struct {
	options map[string]string
	uploads map[string]string
	t       *testing.T
}

func newAzureUpload(t *testing.T, options map[string]string) *azureUpload {
	options[client.StorageProvider] = uploaders.StorageProviderAzure
	return &azureUpload{
		options: options,
		uploads: make(map[string]string),
		t:       t,
	}
}

func (upload *azureUpload) requestUpload(correlationID string, filePath string) map[string]interface{} {
	file := filepath.Base(filePath)
	upload.uploads[correlationID] = file
	return map[string]interface{}{
		paramCorrelationID: correlationID,
		paramOptions:       upload.options,
	}
}

func (upload *azureUpload) download(correlationID string) ([]byte, error) {
	file, ok := upload.uploads[correlationID]
	if !ok {
		return nil, fmt.Errorf("no upload for correlation id: %s", correlationID)
	}
	url := fmt.Sprint(upload.options[uploaders.AzureEndpoint], upload.options[uploaders.AzureContainerName],
		"/", file, "?", upload.options[uploaders.AzureSAS])
	clientOptions := azblob.ClientOptions{}
	blockBlobClient, err := azblob.NewBlockBlobClientWithNoCredential(url, &clientOptions)
	if err != nil {
		return nil, err
	}

	optionsDownload := azblob.DownloadBlobOptions{}
	response, err := blockBlobClient.Download(context.Background(), &optionsDownload)
	if err != nil {
		return nil, err
	}

	bodyStream := response.Body(azblob.RetryReaderOptions{MaxRetryRequests: 3})
	// read the body into a buffer
	downloadedData := bytes.Buffer{}
	_, err = downloadedData.ReadFrom(bodyStream)
	return downloadedData.Bytes(), err
}

func (upload *azureUpload) removeUploads() {
	for _, file := range upload.uploads {
		url := fmt.Sprint(upload.options[uploaders.AzureEndpoint], upload.options[uploaders.AzureContainerName],
			"/", file, "?", upload.options[uploaders.AzureSAS])
		clientOptions := azblob.ClientOptions{}
		blockBlobClient, err := azblob.NewBlockBlobClientWithNoCredential(url, &clientOptions)
		if err == nil {
			optons := azblob.DeleteBlobOptions{}
			blockBlobClient.Delete(context.Background(), &optons)
			upload.t.Logf("successfully deleted %s from azure storage", file)
		} else {
			upload.t.Logf("error deleting %s from azure storage - %v", file, err)
		}
	}
}
