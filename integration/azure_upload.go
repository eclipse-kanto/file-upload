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
	"github.com/stretchr/testify/require"
)

// AzureUpload provides testing functionalities for azure storage provider
type AzureUpload struct {
	options map[string]string
	uploads map[string]string
	t       *testing.T
}

func newAzureUpload(t *testing.T, options map[string]string) *AzureUpload {
	options[client.StorageProvider] = uploaders.StorageProviderAzure
	return &AzureUpload{
		options: options,
		uploads: make(map[string]string),
		t:       t,
	}
}

// NewAzureUpload creates an AzureUpload, retrieving the needed credentials from environment variables
func (suite *FileUploadSuite) NewAzureUpload() *AzureUpload {
	creds, err := uploaders.GetAzureTestCredentials()
	require.NoError(suite.T(), err, "Azure environment variables not set")
	options, err := uploaders.GetAzureTestOptions(creds)
	require.NoError(suite.T(), err, "error getting azure test options")
	return newAzureUpload(suite.T(), options)
}

func (upload *AzureUpload) requestUpload(correlationID string, filePath string) map[string]interface{} {
	file := filepath.Base(filePath)
	upload.uploads[correlationID] = file
	return map[string]interface{}{
		paramCorrelationID: correlationID,
		paramOptions:       upload.options,
	}
}

func (upload *AzureUpload) download(correlationID string) ([]byte, error) {
	url, err := upload.DownloadURL(correlationID)
	if err != nil {
		return nil, err
	}
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

// DownloadURL retrieves the download url for a given correlation id
func (upload *AzureUpload) DownloadURL(correlationID string) (string, error) {
	file, ok := upload.uploads[correlationID]
	if !ok {
		return "", fmt.Errorf(msgNoUploadCorrelationID, correlationID)
	}
	return upload.getURLToFile(file), nil
}

func (upload *AzureUpload) removeUploads() {
	for _, file := range upload.uploads {
		url := upload.getURLToFile(file)
		clientOptions := azblob.ClientOptions{}
		blockBlobClient, err := azblob.NewBlockBlobClientWithNoCredential(url, &clientOptions)
		if err != nil {
			upload.t.Logf("error creating block blob client to azure storage url - %s", url)
			continue
		}
		var deleteResponse azblob.BlobDeleteResponse
		optons := azblob.DeleteBlobOptions{}
		deleteResponse, err = blockBlobClient.Delete(context.Background(), &optons)
		if err != nil {
			upload.t.Logf("deleting blob %s from azure storage finished with error - %v", file, err)
		} else {
			upload.t.Logf("deleting blob %s from azure storage finished with response status - %s", file, deleteResponse.RawResponse.Status)
		}
	}
}

func (upload *AzureUpload) getURLToFile(file string) string {
	return fmt.Sprint(upload.options[uploaders.AzureEndpoint], upload.options[uploaders.AzureContainerName],
		"/", file, "?", upload.options[uploaders.AzureSAS])
}
