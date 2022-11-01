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
)

type awsUpload struct {
	options map[string]string
	uploads map[string]string
	client  *s3.Client
}

func newAWSUpload(options map[string]string) (*awsUpload, error) {
	options[client.StorageProvider] = uploaders.StorageProviderAWS
	client, err := uploaders.GetAWSClient(options)
	if err != nil {
		return nil, err
	}
	return &awsUpload{
		options: options,
		uploads: make(map[string]string),
		client:  client,
	}, nil
}

func (upload *awsUpload) getStartOptions(correlationID string, filePath string) map[string]interface{} {
	upload.uploads[correlationID] = filePath
	return map[string]interface{}{
		paramCorrelationID: correlationID,
		paramOptions:       upload.options,
	}
}

func (upload *awsUpload) getContent(correlationID string) ([]byte, error) {
	filePath, ok := upload.uploads[correlationID]
	if !ok {
		return nil, fmt.Errorf("no upload for correlation id: %s", correlationID)
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

func (upload *awsUpload) cleanup(t *testing.T) {
	for _, filePath := range upload.uploads {
		di := s3.DeleteObjectInput{
			Bucket: aws.String(upload.options[uploaders.AWSBucket]),
			Key:    aws.String(filePath),
		}

		if _, err := upload.client.DeleteObject(context.TODO(), &di); err != nil {
			t.Logf("error deleting %s from aws storage - %v", filePath, err)
		} else {
			t.Logf("successfully deleted %s from aws storage", filePath)
		}
	}
}
