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

package client

import (
	"path/filepath"
	"testing"

	"github.com/eclipse-kanto/file-upload/uploaders"
)

func TestAzureUploadRandomFiles(t *testing.T) {
	testAzureUpload(t, false)
}

func TestAzureUploadEmptyFiles(t *testing.T) {
	testAzureUpload(t, true)
}

func testAzureUpload(t *testing.T, emptyFiles bool) {
	files := createTestFiles(t, 12, true, emptyFiles)
	defer cleanFiles(files)

	options, err := uploaders.GetAzureTestOptions(t)
	assertNoError(t, err)

	options[StorageProvider] = uploaders.StorageProviderAzure
	uploads := NewUploads()
	l := NewTestStatusListener(t)
	paths := getPaths(files)

	ids := uploads.AddMulti("upload-test-azure", paths, true, false, l)
	for ind, id := range ids {
		if u := uploads.Get(id); u != nil {
			err := u.start(options)
			if err != nil {
				t.Fatal(err)
			}
			defer uploaders.DeleteUploadedBlob(t, options, filepath.Base(paths[ind]))
		} else {
			t.Fatalf("upload with ID '%s' not found", id)
		}
	}

	l.waitFinish()
	l.assertStatusState(StateSuccess)
	if l.invalidUploadProgressErrorMessage != "" {
		t.Fatal(l.invalidUploadProgressErrorMessage)
	}
}

func assertNoError(t *testing.T, err error) {
	if err != nil {
		t.Fatal("No error expected", err)
	}
}
