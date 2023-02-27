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
//go:build unit || integration

package uploaders

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/caarlos0/env/v6"
)

// AWSTestCredentials holds credentials for AWS S3 storage
type AWSTestCredentials struct {
	AccessKeyID     string `env:"AWS_ACCESS_KEY_ID"`
	SecretAccessKey string `env:"AWS_SECRET_ACCESS_KEY"`
	Region          string `env:"AWS_REGION"`
	Bucket          string `env:"AWS_BUCKET"`
}

// GetAWSTestCredentials reads aws credentials from environment
func GetAWSTestCredentials() (AWSTestCredentials, error) {
	opts := env.Options{RequiredIfNoDef: true}
	creds := AWSTestCredentials{}
	err := env.Parse(&creds, opts)
	return creds, err
}

// GetAWSTestOptions retrieves the testing options passed to file upload start operation
func GetAWSTestOptions(creds AWSTestCredentials) map[string]string {
	return map[string]string{
		AWSBucket:          creds.Bucket,
		AWSAccessKeyID:     creds.AccessKeyID,
		AWSSecretAccessKey: creds.SecretAccessKey,
		AWSRegion:          creds.Region,
	}
}

// RetrieveAWSTestOptions reads aws credentials from environment and converts them to upload options
func RetrieveAWSTestOptions(t *testing.T) map[string]string {
	t.Helper()

	creds, err := GetAWSTestCredentials()
	if err != nil {
		t.Skipf("Please set azure environment variables(%v).", err)
	}
	return GetAWSTestOptions(creds)
}

// GetAWSClient creates a client to upload, download and delete files from AWS cloud storage
func GetAWSClient(params map[string]string) (*s3.Client, error) {
	cred := credentials.NewStaticCredentialsProvider(params[AWSAccessKeyID], params[AWSSecretAccessKey], "")
	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithCredentialsProvider(cred), config.WithRegion(params[AWSRegion]))

	if err == nil {
		return s3.NewFromConfig(cfg), nil
	}

	return nil, err
}
