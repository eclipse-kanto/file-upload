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

package flags_test

import (
	"flag"
	"os"
	"testing"

	"github.com/eclipse-kanto/file-upload/client"
	flags "github.com/eclipse-kanto/file-upload/flagparse"
	. "github.com/eclipse-kanto/file-upload/flagparsetest"
	"github.com/eclipse-kanto/file-upload/logger"
)

const (
	testConfigFile = "testdata/testConfig.json"
)

var (
	testFileConfig *flags.UploadFileConfig = flags.NewUploadFileConfig()
)

func TestMain(m *testing.M) {
	OriginalArgs = os.Args
	OriginalFlags = *flag.CommandLine

	flags.LoadJSON(testConfigFile, &testFileConfig)
	code := m.Run()

	os.Args = OriginalArgs
	os.Exit(code)
}

func TestOnlyFilesGlob(t *testing.T) {
	ResetFlags()

	SetConfigsDefaults()
	filesGlob := "test"
	PassArgs(&Arg{Name: flags.Files, Value: filesGlob})

	expected := &ParsedFlags{
		BrokerConfig: &DefaultBrokerConfig, UploadConfig: &DefaultUploadConfig,
		LogConfig: &DefaultLogConfig, FilesGlob: filesGlob}

	parseAndVerify(expected, t, !ExpectWarn)
}

func TestCliArgs(t *testing.T) {
	ResetFlags()

	SetTestFullUploadArgs(testFileConfig, AddDefaultsTrue, t)
	PassArgs(TestCliFullArgs...)
	parseAndVerify(FullUploadParsedFlags, t, !ExpectWarn)
}

func TestConfig(t *testing.T) {
	ResetFlags()

	SetTestFullUploadArgs(testFileConfig, AddDefaultsTrue, t)
	PassArgs(&Arg{Name: flags.ConfigFile, Value: testConfigFile})
	parseAndVerify(FullUploadParsedFlags, t, !ExpectWarn)
}

func TestConfigAndCliArgs(t *testing.T) {
	ResetFlags()

	FullUploadParsedFlags = &ParsedFlags{BrokerConfig: &client.BrokerConfig{}, UploadConfig: &client.UploadableConfig{}, LogConfig: &logger.LogConfig{}}
	FullUploadParsedFlags.FilesGlob = testFileConfig.Files

	PassArgs(append(FullUploadParsedFlags.ToArgs(true, t), &Arg{Name: flags.ConfigFile, Value: testConfigFile})...)
	parseAndVerify(FullUploadParsedFlags, t, !ExpectWarn)
}

func TestConfigFileNotExist(t *testing.T) {
	ResetFlags()

	SetConfigsDefaults()
	filesGlob := "test"
	PassArgs(&Arg{Name: flags.ConfigFile, Value: "missingFile"}, &Arg{Name: flags.Files, Value: filesGlob})

	expected := &ParsedFlags{
		BrokerConfig: &DefaultBrokerConfig, UploadConfig: &DefaultUploadConfig,
		LogConfig: &DefaultLogConfig, FilesGlob: filesGlob}

	parseAndVerify(expected, t, ExpectWarn)
}

func SetConfigsDefaults() {
	DefaultFileConfig := flags.GetUploadFileConfigDefaults()
	flags.SetBrokerConfig(DefaultFileConfig, &DefaultBrokerConfig)
	flags.SetUploadableConfig(DefaultFileConfig, &DefaultUploadConfig)
	flags.SetLoggerConfig(DefaultFileConfig, &DefaultLogConfig)
}

func parseAndVerify(expected *ParsedFlags, t *testing.T, expectConfigFileNotFound bool) {
	brokerCfg, uploadCfg, logCfg, glob, err := flags.ParseFlags("n/a")
	actual := &ParsedFlags{
		BrokerConfig: brokerCfg, UploadConfig: uploadCfg,
		LogConfig: logCfg, FilesGlob: glob}

	VerifyEquals(expected, actual, t, nil)
	VerifyNotFoundError(err, expectConfigFileNotFound, t)
}
