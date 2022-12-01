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

// AWSUpload provides testing functionalities for aws storage provider
type AWSUpload struct {
	options       map[string]string
	uploads       map[string]string
	client        *s3.Client
	presignClient *s3.PresignClient
	t             *testing.T
}

func newAWSUpload(t *testing.T, options map[string]string) (*AWSUpload, error) {
	options[client.StorageProvider] = uploaders.StorageProviderAWS
	client, err := uploaders.GetAWSClient(options)
	if err != nil {
		return nil, err
	}
	presignClient := s3.NewPresignClient(client)
	return &AWSUpload{
		options:       options,
		uploads:       make(map[string]string),
		client:        client,
		presignClient: presignClient,
		t:             t,
	}, nil
}

// NewAWSUpload creates an AWSUpload, retrieving the needed credentials from environment variables
func (suite *FileUploadSuite) NewAWSUpload() *AWSUpload {
	creds, err := uploaders.GetAWSTestCredentials()
	require.NoError(suite.T(), err, "AWS environment variables not set")
	options := uploaders.GetAWSTestOptions(creds)
	upload, err := newAWSUpload(suite.T(), options)
	require.NoError(suite.T(), err, "error creating AWS client")
	return upload
}

func (upload *AWSUpload) requestUpload(correlationID string, filePath string) map[string]interface{} {
	upload.uploads[correlationID] = filePath
	return map[string]interface{}{
		paramCorrelationID: correlationID,
		paramOptions:       upload.options,
	}
}

func (upload *AWSUpload) download(correlationID string) ([]byte, error) {
	filePath, ok := upload.uploads[correlationID]
	if !ok {
		return nil, fmt.Errorf(msgNoUploadCorrelationID, correlationID)
	}
	downloader := manager.NewDownloader(upload.client)
	buf := manager.NewWriteAtBuffer([]byte{})
	_, err := downloader.Download(context.TODO(), buf, &s3.GetObjectInput{
		Bucket: aws.String(upload.options[uploaders.AWSBucket]),
		Key:    aws.String(filePath),
	})
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// DownloadURL retrieves the download url for a given correlation id
func (upload *AWSUpload) DownloadURL(correlationID string) (string, error) {
	filePath, ok := upload.uploads[correlationID]
	if !ok {
		return "", fmt.Errorf(msgNoUploadCorrelationID, correlationID)
	}
	request, err := upload.presignClient.PresignGetObject(context.TODO(), &s3.GetObjectInput{
		Bucket: aws.String(upload.options[uploaders.AWSBucket]),
		Key:    aws.String(filePath),
	})
	if err != nil {
		return "", err
	}
	return request.URL, nil
}

func (upload *AWSUpload) removeUploads() {
	for _, filePath := range upload.uploads {
		di := s3.DeleteObjectInput{
			Bucket: aws.String(upload.options[uploaders.AWSBucket]),
			Key:    aws.String(filePath),
		}

		if _, err := upload.client.DeleteObject(context.TODO(), &di); err != nil {
			upload.t.Logf("error deleting %s from aws storage - %v", filePath, err)
		} else {
			upload.t.Logf("successfully deleted %s from aws storage", filePath)
		}
	}
}
