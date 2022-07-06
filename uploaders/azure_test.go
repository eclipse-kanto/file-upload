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
	"bytes"
	"context"
	"fmt"
	"log"
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
	options := getAzureTestOptions(t)
	u, err := NewAzureUploader(options)
	assertNoError(t, err)

	f, err := os.Open(testFile)
	assertNoError(t, err)
	defer f.Close()

	err = u.UploadFile(f, useChecksum)
	assertNoError(t, err)

	urlStr := fmt.Sprint(options[AzureEndpoint], options[AzureContainerName], "/", testFile, "?", options[AzureSas])
	clientOptions := azblob.ClientOptions{}
	blockBlobClient, err := azblob.NewBlockBlobClientWithNoCredential(urlStr, &clientOptions)
	defer deleteBlob(blockBlobClient)
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
	options := getAzureTestOptions(t)
	requiredParams := []string{AzureContainerName, AzureEndpoint, AzureSas}

	for _, param := range requiredParams {
		options := partialCopy(options, param)
		u, err := NewAzureUploader(options)
		assertFailsWith(t, u, err, fmt.Sprintf(missingParameterErrMsg, param))
	}

}

func deleteBlob(blockBlobClient azblob.BlockBlobClient) {
	optons := azblob.DeleteBlobOptions{}
	_, err := blockBlobClient.Delete(context.Background(), &optons)
	if err != nil {
		log.Println(err)
	}
}

func getAzureTestCredentials(t *testing.T) AzureTestCredentials {
	t.Helper()

	accountNameKey, containerNameKey, tenantIDKey, clientIDKey, clientSecretKey :=
		"AZURE_ACCOUNT_NAME", "AZURE_CONTAINER_NAME", "AZURE_TENANT_ID", "AZURE_CLIENT_ID", "AZURE_CLIENT_SECRET"
	mapping := map[string]string{
		accountNameKey:   "",
		containerNameKey: "",
		tenantIDKey:      "",
		clientIDKey:      "",
		clientSecretKey:  "",
	}

	for key := range mapping {
		env := os.Getenv(key)
		if env == "" {
			t.Skipf("environment variable '%s' not set", key)
		} else {
			mapping[key] = env
		}
	}

	return AzureTestCredentials{
		accountName:   mapping[accountNameKey],
		containerName: mapping[containerNameKey],
		tenantID:      mapping[tenantIDKey],
		clientID:      mapping[clientIDKey],
		clientSecret:  mapping[clientSecretKey],
	}
}

func getAzureTestOptions(t *testing.T) map[string]string {
	t.Helper()

	azureTestCredentials := getAzureTestCredentials(t)
	azureSas, err := getOneHourAzureSas(t, azureTestCredentials)
	assertNoError(t, err)
	return map[string]string{
		AzureEndpoint:      fmt.Sprintf(azureURLPattern, azureTestCredentials.accountName),
		AzureSas:           azureSas,
		AzureContainerName: azureTestCredentials.containerName,
	}
}
