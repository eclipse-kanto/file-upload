// Copyright (c) 2021 Contributors to the Eclipse Foundation
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

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go/logging"
	"github.com/eclipse-kanto/file-upload/logger"
)

// Constants for AWS upload 'start' operation options
const (
	StorageProviderAWS = "aws"

	AWSRegion          = "aws.region"
	AWSAccessKeyID     = "aws.access.key.id"
	AWSSecretAccessKey = "aws.secret.access.key"
	AWSSessionToken    = "aws.session.token"
	AWSBucket          = "aws.s3.bucket"
	AWSObjectKey       = "aws.object.key"
)

// AWSUploader handles upload to AWS S3 storage
type AWSUploader struct {
	bucket    string
	objectKey string

	uploader *manager.Uploader
}

type awsCredentials struct {
	key    string
	secret string
	token  string
	region string
	bucket string
}

type awsLogger struct{}

func (l *awsLogger) Logf(classification logging.Classification, format string, v ...interface{}) {
	if classification == logging.Debug {
		logger.Debugf(format, v...)
	} else if classification == logging.Warn {
		logger.Warnf(format, v...)
	} else {
		logger.Infof(format, v...)
	}
}

// NewAWSUploader construct new AWSUploader from the provided 'start' operation options
func NewAWSUploader(options map[string]string) (Uploader, error) {
	cred, err := getAWSCredentials(options)

	if err != nil {
		return nil, err
	}

	var logMode aws.ClientLogMode
	if logger.IsDebugEnabled() {
		logMode = aws.LogRequest | aws.LogResponse | aws.LogRetries
	}

	provider := credentials.NewStaticCredentialsProvider(cred.key, cred.secret, cred.token)
	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithCredentialsProvider(provider),
		config.WithRegion(cred.region),
		config.WithLogger(&awsLogger{}),
		config.WithClientLogMode(logMode),
	)

	if err != nil {
		return nil, err
	}

	uploader := manager.NewUploader(s3.NewFromConfig(cfg))
	objectKey := options[AWSObjectKey]

	return &AWSUploader{cred.bucket, objectKey, uploader}, nil
}

// UploadFile performs AWS S3 file upload
func (u *AWSUploader) UploadFile(file *os.File, useChecksum bool, listener func(bytesTransferred int64)) error {
	name := u.objectKey
	if u.objectKey == "" {
		name = file.Name()
	}

	var md5 string
	if useChecksum {
		hash, err := ComputeMD5(file, true)
		if err != nil {
			return err
		}
		md5 = hash
	}

	_, err := u.uploader.Upload(context.Background(), &s3.PutObjectInput{
		Bucket:     &u.bucket,
		Key:        aws.String(name),
		Body:       file,
		ContentMD5: &md5,
	})

	return err
}

func getAWSCredentials(options map[string]string) (*awsCredentials, error) {
	r := &awsCredentials{}

	r.bucket = options[AWSBucket]
	r.key = options[AWSAccessKeyID]
	r.region = options[AWSRegion]
	r.secret = options[AWSSecretAccessKey]
	r.token = options[AWSSessionToken]

	if r.bucket == "" {
		return nil, fmt.Errorf(missingParameterErrMsg, AWSBucket)
	}

	if r.key == "" {
		return nil, fmt.Errorf(missingParameterErrMsg, AWSAccessKeyID)
	}

	if r.region == "" {
		return nil, fmt.Errorf(missingParameterErrMsg, AWSRegion)
	}

	if r.secret == "" {
		return nil, fmt.Errorf(missingParameterErrMsg, AWSSecretAccessKey)
	}

	//token is optional

	return r, nil
}
