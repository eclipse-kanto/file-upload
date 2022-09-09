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

package flags_test

import (
	"os"
	"testing"

	flags "github.com/eclipse-kanto/file-upload/flagparse"
	. "github.com/eclipse-kanto/file-upload/flagparsetest"
)

const (
	testConfigFile = "testdata/testConfig.json"
)

var (
	testConfig flags.UploadConfig = flags.UploadConfig{}
)

func TestMain(m *testing.M) {
	originalArgs := os.Args

	flags.LoadJSON(testConfigFile, &testConfig)
	code := m.Run()

	os.Args = originalArgs
	os.Exit(code)
}

func TestOnlyFilesGlob(t *testing.T) {
	ResetFlags()

	filesGlob := "test"
	PassArgs(Arg{Name: flags.Files, Value: filesGlob})

	expected := getDefaultConfig()

	parseAndVerify(expected, t, false)
}

func TestCliArgs(t *testing.T) {
	ResetFlags()

	args := ConfigToArgs(t, &testConfig, nil, true)
	PassArgs(args...)
	parseAndVerify(&testConfig, t, false)
}

func TestConfig(t *testing.T) {
	ResetFlags()

	PassArgs(Arg{Name: flags.ConfigFile, Value: testConfigFile})
	parseAndVerify(&testConfig, t, false)
}

func TestConfigAndCliArgs(t *testing.T) {
	ResetFlags()

	config := &flags.UploadConfig{}
	config.Files = testConfig.Files

	args := ConfigToArgs(t, config, nil, true)
	args = append(args, Arg{Name: flags.ConfigFile, Value: testConfigFile})

	PassArgs(args...)
	parseAndVerify(config, t, false)
}

func TestConfigFileNotExist(t *testing.T) {
	ResetFlags()

	filesGlob := "test"
	PassArgs(Arg{Name: flags.ConfigFile, Value: "missingFile"}, Arg{Name: flags.Files, Value: filesGlob})

	expected := getDefaultConfig()

	parseAndVerify(expected, t, true)
}

func getDefaultConfig() *flags.UploadConfig {
	cfg := &flags.UploadConfig{}

	flags.InitConfigDefaults(cfg, flags.ConfigNames, nil)
	cfg.Files = "test"

	return cfg
}

func parseAndVerify(expected *flags.UploadConfig, t *testing.T, expectConfigFileNotFound bool) {
	parsed, err := flags.ParseFlags("n/a")

	VerifyEquals(expected, parsed, t, nil)
	VerifyNotFoundError(err, expectConfigFileNotFound, t)
}
