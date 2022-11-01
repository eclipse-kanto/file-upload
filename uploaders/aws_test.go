// Copyright (c) 2021 Contributors to the Eclipse Foundation
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
	"fmt"
	"log"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func TestAWSUploadWithoutChecksum(t *testing.T) {
	testAWSUpload(t, false)
}

func TestAWSUploadWithChecksum(t *testing.T) {
	testAWSUpload(t, true)
}

func testAWSUpload(t *testing.T, useChecksum bool) {
	creds := GetAWSTestOptions(t)

	client, err := GetAWSClient(creds)
	assertNoError(t, err)

	u, err := NewAWSUploader(creds)
	assertNoError(t, err)

	f, err := os.Open(testFile)
	assertNoError(t, err)
	defer f.Close()

	err = u.UploadFile(f, useChecksum, nil)
	assertNoError(t, err)

	defer deleteAWSObject(client, testFile, creds[AWSBucket])

	downloader := manager.NewDownloader(client)
	buf := manager.NewWriteAtBuffer([]byte{})
	_, err = downloader.Download(context.TODO(), buf, &s3.GetObjectInput{
		Bucket: aws.String(creds[AWSBucket]),
		Key:    aws.String(testFile),
	})
	assertNoError(t, err)

	assertStringsSame(t, "test file content", string(buf.Bytes()), testBody)
}

func TestNewAWSUploaderErrors(t *testing.T) {
	creds := GetAWSTestOptions(t)

	requiredParams := []string{AWSAccessKeyID, AWSBucket, AWSSecretAccessKey, AWSRegion}

	for _, param := range requiredParams {
		options := partialCopy(creds, param)
		u, err := NewAWSUploader(options)
		assertFailsWith(t, u, err, fmt.Sprintf(missingParameterErrMsg, param))
	}

}

func deleteAWSObject(client *s3.Client, key string, bucket string) {
	di := s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}

	if _, err := client.DeleteObject(context.TODO(), &di); err != nil {
		log.Println(err)
	}
}
