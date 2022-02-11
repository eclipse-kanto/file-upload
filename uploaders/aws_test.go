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
	"log"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
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
	creds := getTestCredentials(t)

	client, err := getAWSClient(creds)
	assertNoError(t, err)

	u, err := NewAWSUploader(creds)
	assertNoError(t, err)

	f, err := os.Open(testFile)
	assertNoError(t, err)
	defer f.Close()

	md5 := getChecksum(t, f, useChecksum)
	err = u.UploadFile(f, md5)
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
	creds := getTestCredentials(t)

	requiredParams := []string{AWSAccessKeyID, AWSBucket, AWSSecretAccessKey, AWSRegion}

	for _, param := range requiredParams {
		options := partialCopy(creds, param)
		u, err := NewAWSUploader(options)
		assertFailsWith(t, u, err, fmt.Sprintf(missingParameterErrMsg, param))
	}

}

func assertFailsWith(t *testing.T, u interface{}, err error, msg string) {
	t.Helper()

	assertNil(t, u)

	if err == nil {
		t.Fatalf("error '%s' expected", msg)
	}

	if err.Error() != msg {
		t.Fatalf("expected error '%s', but was '%v'", msg, err)
	}
}

func getTestCredentials(t *testing.T) map[string]string {
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

func getAWSClient(params map[string]string) (*s3.Client, error) {
	cred := credentials.NewStaticCredentialsProvider(params[AWSAccessKeyID], params[AWSSecretAccessKey], "")
	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithCredentialsProvider(cred), config.WithRegion(params[AWSRegion]))

	if err == nil {
		return s3.NewFromConfig(cfg), nil
	}

	return nil, err
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

func partialCopy(m map[string]string, omit string) map[string]string {
	c := map[string]string{}

	for k, v := range m {
		if k != omit {
			c[k] = v
		}
	}

	return c
}
