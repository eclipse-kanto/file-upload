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
)

// AzureTestCredentials holds credentials for Azure Blob upload
type AzureTestCredentials struct {
	accountName, containerName, clientID, tenantID, clientSecret string
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
		fmt.Sprintf(blobPath, azureTestCredentials.accountName, azureTestCredentials.containerName),
		*udk.SignedOid,
		azureTestCredentials.tenantID,
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

func getOneHourAzureSas(t *testing.T, azureTestCredentials AzureTestCredentials) (string, error) {
	t.Helper()

	options := azidentity.ClientSecretCredentialOptions{}
	credential, err := azidentity.NewClientSecretCredential(azureTestCredentials.tenantID, azureTestCredentials.clientID,
		azureTestCredentials.clientSecret, &options)
	if err != nil {
		return "", nil
	}
	startTime := time.Now().UTC().Format(sasTimeFormat)
	expiryTime := time.Now().Add(time.Hour).UTC().Format(sasTimeFormat)
	keyInfo := azblob.KeyInfo{
		Start:  &startTime,
		Expiry: &expiryTime,
	}
	udk, err := requestUserDelegationKey(fmt.Sprintf(azureURLPattern, azureTestCredentials.accountName), keyInfo, credential)
	if err != nil {
		return "", nil
	}
	signature, err := generateSignarute(udk, azureTestCredentials, keyInfo)
	if err != nil {
		return "", nil
	}
	return generateEncodedSas(udk, azureTestCredentials, keyInfo, signature), nil
}

func generateEncodedSas(udk azblob.UserDelegationKey, azureTestCredentials AzureTestCredentials, keyInfo azblob.KeyInfo, signature string) string {
	result := bytes.Buffer{}
	appendToQuery(&result, "sv", *udk.SignedVersion)
	appendToQuery(&result, "se", *keyInfo.Expiry)
	appendToQuery(&result, "skoid", *udk.SignedOid)
	appendToQuery(&result, "sktid", azureTestCredentials.tenantID)
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
