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

package uploaders

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
)

func TestAzureUploadWithoutChecksum(t *testing.T) {
	testAzureUpload(t, false)
}

func TestAzureUploadWithChecksum(t *testing.T) {
	testAzureUpload(t, true)
}

func testAzureUpload(t *testing.T, useChecksum bool) {
	options, err := GetAzureTestOptions(t)
	assertNoError(t, err)
	u, err := NewAzureUploader(options)
	assertNoError(t, err)

	f, err := os.Open(testFile)
	assertNoError(t, err)
	defer f.Close()

	err = u.UploadFile(f, useChecksum, nil)
	assertNoError(t, err)

	urlStr := fmt.Sprint(options[AzureEndpoint], options[AzureContainerName], "/", testFile, "?", options[AzureSAS])
	clientOptions := azblob.ClientOptions{}
	blockBlobClient, err := azblob.NewBlockBlobClientWithNoCredential(urlStr, &clientOptions)
	defer deleteBlob(t, blockBlobClient)
	assertNoError(t, err)

	optionsDownload := azblob.DownloadBlobOptions{}
	response, err := blockBlobClient.Download(context.Background(), &optionsDownload)
	assertNoError(t, err)

	bodyStream := response.Body(azblob.RetryReaderOptions{MaxRetryRequests: 3})
	// read the body into a buffer
	downloadedData := bytes.Buffer{}
	n, err := downloadedData.ReadFrom(bodyStream)
	assertNoError(t, err)
	assertEquals(t, "Wrong number of downloaded bytes", int64(len(testBody)), n)
	assertStringsSame(t, "Test file content", testBody, string(downloadedData.Bytes()))
}

func TestNewAzureUploaderErrors(t *testing.T) {
	options, err := GetAzureTestOptions(t)
	assertNoError(t, err)
	requiredParams := []string{AzureContainerName, AzureEndpoint, AzureSAS}

	for _, param := range requiredParams {
		options := partialCopy(options, param)
		u, err := NewAzureUploader(options)
		assertFailsWith(t, u, err, fmt.Sprintf(missingParameterErrMsg, param))
	}

}

func deleteBlob(t *testing.T, blockBlobClient azblob.BlockBlobClient) {
	t.Helper()

	optons := azblob.DeleteBlobOptions{}
	_, err := blockBlobClient.Delete(context.Background(), &optons)
	if err != nil {
		t.Log(err)
	}
}
