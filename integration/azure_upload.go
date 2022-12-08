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

// azureStorageProvider provides testing functionalities for the Azure storage provider
type azureStorageProvider struct {
	options map[string]string
	uploads map[string]string
	t       *testing.T
}

// newAzureStorageProvider creates an implementation of the storageProvider interface for the Azure storage provider,
// retrieves the needed credentials from environment variables
func newAzureStorageProvider(t *testing.T) storageProvider {
	creds, err := uploaders.GetAzureTestCredentials()
	require.NoError(t, err, "Azure credentials not set")
	options, err := uploaders.GetAzureTestOptions(creds)
	require.NoError(t, err, "error getting azure test options")
	options[client.StorageProvider] = uploaders.StorageProviderAzure
	return azureStorageProvider{
		options: options,
		uploads: make(map[string]string),
		t:       t,
	}
}

func (provider azureStorageProvider) requestUpload(correlationID string, filePath string) map[string]interface{} {
	file := filepath.Base(filePath)
	provider.uploads[correlationID] = file
	return map[string]interface{}{
		paramCorrelationID: correlationID,
		paramOptions:       provider.options,
	}
}

func (provider azureStorageProvider) download(correlationID string) ([]byte, error) {
	url, err := provider.downloadURL(correlationID)
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

func (provider azureStorageProvider) downloadURL(correlationID string) (string, error) {
	file, ok := provider.uploads[correlationID]
	if !ok {
		return "", fmt.Errorf(msgNoUploadCorrelationID, correlationID)
	}
	return provider.urlToFile(file), nil
}

func (provider azureStorageProvider) removeUploads() {
	for _, file := range provider.uploads {
		url := provider.urlToFile(file)
		clientOptions := azblob.ClientOptions{}
		blockBlobClient, err := azblob.NewBlockBlobClientWithNoCredential(url, &clientOptions)
		if err != nil {
			provider.t.Logf("error creating block blob client to azure storage url - %s", url)
			continue
		}
		var deleteResponse azblob.BlobDeleteResponse
		optons := azblob.DeleteBlobOptions{}
		deleteResponse, err = blockBlobClient.Delete(context.Background(), &optons)
		if err != nil {
			provider.t.Logf("deleting blob %s from azure storage finished with error - %v", file, err)
		} else {
			provider.t.Logf("deleting blob %s from azure storage finished with response status - %s", file, deleteResponse.RawResponse.Status)
		}
	}
}

func (provider azureStorageProvider) urlToFile(file string) string {
	return fmt.Sprint(provider.options[uploaders.AzureEndpoint], provider.options[uploaders.AzureContainerName],
		"/", file, "?", provider.options[uploaders.AzureSAS])
}
