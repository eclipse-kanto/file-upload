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
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/stretchr/testify/require"
)

// CheckDirIsEmpty checks if a directory is empty
func (suite *FileUploadSuite) CheckDirIsEmpty(dir string) {
	files, err := os.ReadDir(dir)
	if err != nil {
		suite.T().Fatalf("upload dir %s cannot be read - %v", dir, err)
	}
	if len(files) > 0 {
		suite.T().Fatalf("upload dir %s must be empty", dir)
	}
}

// CreateTestFiles creates a given number of files in a given directory, filling the with some test bytes
func CreateTestFiles(dir string, fileCount int) ([]string, error) {
	var result []string
	for i := 1; i <= fileCount; i++ {
		filePath := filepath.Join(dir, fmt.Sprintf(uploadFilesPattern, i))
		result = append(result, filePath)
		if err := writeTestContent(filePath, 10*i); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func writeTestContent(filePath string, count int) error {
	data := strings.Repeat("test", count)
	return os.WriteFile(filePath, []byte(data), fs.ModePerm)
}

// RemoveFiles removes all files from a given directory
func (suite *FileUploadSuite) RemoveFiles(dir string) {
	files, err := os.ReadDir(dir)
	if err != nil {
		suite.T().Logf("error reading files from directory %s(%v)", dir, err)
	}
	for _, file := range files {
		path := filepath.Join(dir, file.Name())
		if err := os.Remove(path); err != nil {
			suite.T().Logf("error cleaning file %s(%v)", path, err)
		}
	}
}

// CompareContent compares the content of a file with the actual bytes
func (suite *FileUploadSuite) CompareContent(filePath string, received []byte) {
	expected, err := os.ReadFile(filePath)
	require.NoErrorf(suite.T(), err, "cannot read file %s", filePath)
	require.Equalf(suite.T(), string(expected), string(received), "uploaded/restored content of file %s differs from original", filePath)
}
