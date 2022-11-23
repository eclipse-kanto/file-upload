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
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/caarlos0/env/v6"
)

// AzureTestCredentials holds credentials for Azure Blob upload
type AzureTestCredentials struct {
	AccountName   string `env:"AZURE_ACCOUNT_NAME"`
	ContainerName string `env:"AZURE_CONTAINER_NAME"`
	ClientID      string `env:"AZURE_CLIENT_ID"`
	TenantID      string `env:"AZURE_TENANT_ID"`
	ClientSecret  string `env:"AZURE_CLIENT_SECRET"`
}

const (
	azureURLPattern      = "https://%s.blob.core.windows.net/"
	sasTimeFormat        = "2006-01-02T15:04:05Z"
	requestModule        = "azblob"
	requestModuleVersion = "v0.2.0"
	defaultTokenScope    = "https://storage.azure.com/.default"
	sasPermissions       = "racwdl"
	blobPath             = "/blob/%s/%s"
	service              = "b" // blob
	resource             = "c" // container
)

func requestUserDelegationKey(endpoint string, keyInfo azblob.KeyInfo, credential azcore.TokenCredential) (azblob.UserDelegationKey, error) {
	var udk azblob.UserDelegationKey
	perRetryPolicies := []policy.Policy{
		runtime.NewBearerTokenPolicy(credential, []string{defaultTokenScope}, nil),
	}
	options := azcore.ClientOptions{}
	pipeline := runtime.NewPipeline(requestModule, requestModuleVersion, nil, perRetryPolicies, &options)
	request, err := runtime.NewRequest(context.Background(), http.MethodPost, endpoint)
	if err != nil {
		return udk, err
	}
	reqQP := request.Raw().URL.Query()
	reqQP.Set("restype", "service")
	reqQP.Set("comp", "userdelegationkey")
	request.Raw().URL.RawQuery = reqQP.Encode()
	request.Raw().Header.Set("x-ms-version", "2020-04-08")
	request.Raw().Header.Set("Accept", "application/xml")
	err = runtime.MarshalAsXML(request, keyInfo)
	if err != nil {
		return udk, err
	}
	response, err := pipeline.Do(request)
	if err != nil {
		return udk, err
	}
	return udk, runtime.UnmarshalAsXML(response, &udk)
}

func generateSignarute(udk azblob.UserDelegationKey, azureTestCredentials AzureTestCredentials, keyInfo azblob.KeyInfo) (string, error) {
	stringToSign := strings.Join([]string{
		sasPermissions,
		"", // empty startTime
		*keyInfo.Expiry,
		fmt.Sprintf(blobPath, azureTestCredentials.AccountName, azureTestCredentials.ContainerName),
		*udk.SignedOid,
		azureTestCredentials.TenantID,
		*keyInfo.Start,
		*keyInfo.Expiry,
		service,
		*udk.SignedVersion,
		"", // empty authorized object id
		"", // empty suoid
		"", // empty correlation id
		"", // empty IPRange
		"", // empty Protocol
		*udk.SignedVersion,
		resource,
		"", // empty snapshot timestamp
		"", // empty cache control
		"", // empty content disposition
		"", // empty content encoding
		"", // empty content language
		"", // empty content type
	}, "\n")

	signingKey, err := base64.StdEncoding.DecodeString(*udk.Value)
	if err != nil {
		return "", err
	}
	h := hmac.New(sha256.New, signingKey)
	_, err = h.Write([]byte(stringToSign))
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(h.Sum(nil)), nil
}

func getOneHourAzureSAS(azureTestCredentials AzureTestCredentials) (string, error) {
	options := azidentity.ClientSecretCredentialOptions{}
	credential, err := azidentity.NewClientSecretCredential(azureTestCredentials.TenantID, azureTestCredentials.ClientID,
		azureTestCredentials.ClientSecret, &options)
	if err != nil {
		return "", err
	}
	startTime := time.Now().UTC().Format(sasTimeFormat)
	expiryTime := time.Now().Add(time.Hour).UTC().Format(sasTimeFormat)
	keyInfo := azblob.KeyInfo{
		Start:  &startTime,
		Expiry: &expiryTime,
	}
	udk, err := requestUserDelegationKey(fmt.Sprintf(azureURLPattern, azureTestCredentials.AccountName), keyInfo, credential)
	if err != nil {
		return "", err
	}
	signature, err := generateSignarute(udk, azureTestCredentials, keyInfo)
	if err != nil {
		return "", err
	}
	return generateEncodedSAS(udk, azureTestCredentials, keyInfo, signature), nil
}

func generateEncodedSAS(udk azblob.UserDelegationKey, azureTestCredentials AzureTestCredentials, keyInfo azblob.KeyInfo, signature string) string {
	result := bytes.Buffer{}
	appendToQuery(&result, "sv", *udk.SignedVersion)
	appendToQuery(&result, "se", *keyInfo.Expiry)
	appendToQuery(&result, "skoid", *udk.SignedOid)
	appendToQuery(&result, "sktid", azureTestCredentials.TenantID)
	appendToQuery(&result, "skt", *keyInfo.Start)
	appendToQuery(&result, "ske", *keyInfo.Expiry)
	appendToQuery(&result, "sks", service)
	appendToQuery(&result, "skv", *udk.SignedVersion)
	appendToQuery(&result, "sr", resource)
	appendToQuery(&result, "sp", sasPermissions)
	appendToQuery(&result, "sig", signature)
	return result.String()
}

func appendToQuery(result *bytes.Buffer, key, val string) {
	if result.Len() > 0 {
		result.WriteRune('&')
	}
	result.WriteString(key + "=" + url.QueryEscape(val))
}

// GetAzureTestCredentials reads azure credentials from environment
func GetAzureTestCredentials() (AzureTestCredentials, error) {
	opts := env.Options{RequiredIfNoDef: true}
	creds := AzureTestCredentials{}
	err := env.Parse(&creds, opts)
	return creds, err
}

// GetAzureTestOptions retrieves the testing options passed to file upload start operation
func GetAzureTestOptions(creds AzureTestCredentials) (map[string]string, error) {
	azureSAS, err := getOneHourAzureSAS(creds)
	if err != nil {
		return nil, err
	}
	return map[string]string{
		AzureEndpoint:      fmt.Sprintf(azureURLPattern, creds.AccountName),
		AzureSAS:           azureSAS,
		AzureContainerName: creds.ContainerName,
	}, nil
}

// RetrieveAzureTestOptions reads azure credentials from environment and converts them to upload options
func RetrieveAzureTestOptions(t *testing.T) map[string]string {
	t.Helper()

	creds, err := GetAzureTestCredentials()
	if err != nil {
		t.Skipf("Please set azure environment variables(%v).", err)
	}
	options, err := GetAzureTestOptions(creds)
	if err != nil {
		t.Error(err)
	}
	return options
}

// DeleteUploadedBlob deletes an uploaded blob from azure storage
func DeleteUploadedBlob(t *testing.T, options map[string]string, filename string) {
	t.Helper()

	urlStr := fmt.Sprint(options[AzureEndpoint], options[AzureContainerName], "/", filename, "?", options[AzureSAS])
	clientOptions := azblob.ClientOptions{}
	blockBlobClient, err := azblob.NewBlockBlobClientWithNoCredential(urlStr, &clientOptions)
	if err != nil {
		t.Logf("cannot cleanup azure blob %s - %v", urlStr, err)
		return
	}

	opts := azblob.DeleteBlobOptions{}
	_, err = blockBlobClient.Delete(context.Background(), &opts)
	if err != nil {
		t.Logf("cannot cleanup azure blob %s - %v", urlStr, err)
	}
}
