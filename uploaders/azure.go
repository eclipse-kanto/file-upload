// Copyright (c) 2022 Contributors to the Eclipse Foundation
//
// See the NOTICE file(s) distributed with this work for additional
// information regarding copyright ownership.
//
// This program and the accompanying materials are made available under the
// terms of the Eclipse Public License 2.0 which is available at
// http://www.eclipse.org/legal/epl-2.0
//
// SPDX-License-Identifier: EPL-2.0

package uploaders

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
)

// Constants for Azure upload 'start' operation options
const (
	StorageProviderAzure = "azure"

	AzureEndpoint      = "azure.storage.endpoint"
	AzureSas           = "azure.shared.access.signature"
	AzureContainerName = "azure.blob.container"
)

// AzureUploader handles upload to Azure Blob storage
type AzureUploader struct {
	endpoint  string
	sas       string
	container string
}

// NewAzureUploader constructs new AWSUploader from provided 'start' operation options
func NewAzureUploader(options map[string]string) (Uploader, error) {
	uploader := &AzureUploader{
		endpoint:  options[AzureEndpoint],
		sas:       options[AzureSas],
		container: options[AzureContainerName],
	}
	if uploader.endpoint == "" {
		return nil, fmt.Errorf(missingParameterErrMsg, AzureEndpoint)
	}
	if uploader.sas == "" {
		return nil, fmt.Errorf(missingParameterErrMsg, AzureSas)
	}
	if uploader.container == "" {
		return nil, fmt.Errorf(missingParameterErrMsg, AzureContainerName)
	}
	return uploader, nil
}

// UploadFile performs Azure file upload
func (u *AzureUploader) UploadFile(file *os.File, useChecksum bool) error {
	clientOptions := azblob.ClientOptions{}
	blockBlobClient, err := azblob.NewBlockBlobClientWithNoCredential(fmt.Sprint(u.endpoint, u.container, "/", filepath.Base(file.Name()), "?", u.sas), &clientOptions)
	if err != nil {
		return err
	}

	blobHTTPHeaders := &azblob.BlobHTTPHeaders{}
	if useChecksum {
		md5, err := ComputeMD5(file, false)
		if err != nil {
			return err
		}
		blobHTTPHeaders.BlobContentMD5 = []byte(md5)
	}
	options := azblob.HighLevelUploadToBlockBlobOption{
		HTTPHeaders: blobHTTPHeaders,
	}

	response, err := blockBlobClient.UploadFileToBlockBlob(context.Background(), file, options) // perform upload
	if err == nil {
		if response.StatusCode/100 != 2 {
			return fmt.Errorf("unsuccessful response status code - %v", response.StatusCode)
		}
	}
	return err
}
