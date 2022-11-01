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
	"context"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// GetAWSTestOptions retrieves the testing options passed to file upload start operation
func GetAWSTestOptions(t *testing.T) map[string]string {
	t.Helper()

	mapping := map[string]string{
		"AWS_BUCKET":            AWSBucket,
		"AWS_ACCESS_KEY_ID":     AWSAccessKeyID,
		"AWS_SECRET_ACCESS_KEY": AWSSecretAccessKey,
		"AWS_REGION":            AWSRegion,
	}

	creds := map[string]string{}
	for k, v := range mapping {
		env := os.Getenv(k)
		if env != "" {
			creds[v] = env
		} else {
			t.Skipf("environment variable '%s' not set", k)
		}
	}

	return creds
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
