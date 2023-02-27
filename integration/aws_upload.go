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

//go:build integration

package integration

import (
	"context"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/eclipse-kanto/file-upload/client"
	"github.com/eclipse-kanto/file-upload/uploaders"
	"github.com/stretchr/testify/require"
)

// awsStorageProvider provides testing functionalities for the AWS storage provider
type awsStorageProvider struct {
	options       map[string]string
	uploads       map[string]string
	client        *s3.Client
	presignClient *s3.PresignClient
	t             *testing.T
}

// newAWSStorageProvider creates an implementation of the storageProvider interface for the AWS storage provider,
// retrieves the needed credentials from environment variables
func newAWSStorageProvider(t *testing.T) storageProvider {
	creds, err := uploaders.GetAWSTestCredentials()
	require.NoError(t, err, "AWS credentials not set")
	options := uploaders.GetAWSTestOptions(creds)
	options[client.StorageProvider] = uploaders.StorageProviderAWS
	client, err := uploaders.GetAWSClient(options)
	require.NoError(t, err, "error getting AWS client")
	presignClient := s3.NewPresignClient(client)
	return awsStorageProvider{
		options:       options,
		uploads:       make(map[string]string),
		client:        client,
		presignClient: presignClient,
		t:             t,
	}
}

func (provider awsStorageProvider) requestUpload(correlationID string, filePath string) map[string]interface{} {
	provider.uploads[correlationID] = filePath
	return map[string]interface{}{
		paramCorrelationID: correlationID,
		paramOptions:       provider.options,
	}
}

func (provider awsStorageProvider) download(correlationID string) ([]byte, error) {
	filePath, ok := provider.uploads[correlationID]
	if !ok {
		return nil, fmt.Errorf(msgNoUploadCorrelationID, correlationID)
	}
	downloader := manager.NewDownloader(provider.client)
	buf := manager.NewWriteAtBuffer([]byte{})
	_, err := downloader.Download(context.TODO(), buf, &s3.GetObjectInput{
		Bucket: aws.String(provider.options[uploaders.AWSBucket]),
		Key:    aws.String(filePath),
	})
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (provider awsStorageProvider) downloadURL(correlationID string) (string, error) {
	filePath, ok := provider.uploads[correlationID]
	if !ok {
		return "", fmt.Errorf(msgNoUploadCorrelationID, correlationID)
	}
	request, err := provider.presignClient.PresignGetObject(context.TODO(), &s3.GetObjectInput{
		Bucket: aws.String(provider.options[uploaders.AWSBucket]),
		Key:    aws.String(filePath),
	})
	if err != nil {
		return "", err
	}
	return request.URL, nil
}

func (provider awsStorageProvider) removeUploads() {
	for _, filePath := range provider.uploads {
		di := s3.DeleteObjectInput{
			Bucket: aws.String(provider.options[uploaders.AWSBucket]),
			Key:    aws.String(filePath),
		}

		if _, err := provider.client.DeleteObject(context.TODO(), &di); err != nil {
			provider.t.Logf("error deleting %s from AWS storage - %v", filePath, err)
		} else {
			provider.t.Logf("successfully deleted %s from AWS storage", filePath)
		}
	}
}
