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

package flagstestutil

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"testing"
	"time"
	"unicode"

	"github.com/eclipse-kanto/file-upload/client"
	flags "github.com/eclipse-kanto/file-upload/flagparse"
	"github.com/eclipse-kanto/file-upload/logger"
)

// ParsedFlags represents parsed cli flags
type ParsedFlags struct {
	BrokerConfig *client.BrokerConfig
	UploadConfig *client.UploadableConfig
	LogConfig    *logger.LogConfig
	FilesGlob    string
}

// Arg represents cli args
type Arg struct {
	Name  string
	Value string
}

func (arg *Arg) String() string {
	return fmt.Sprintf("-%v=%v", arg.Name, arg.Value)
}

// Common test values
const (
	defaultMessageFormat = "Expected differs from actual:\nexpected:\n%v\nactual:\n%v:"
	ExpectWarn           = true
	AddDefaultsFalse     = false
	AddDefaultsTrue      = true
)

// Common test variables
var (
	OriginalArgs  []string
	OriginalFlags flag.FlagSet

	TestCliFullArgs       []*Arg
	FullUploadParsedFlags *ParsedFlags

	// used in actual test cases, put here to avoid duplication
	DefaultBrokerConfig client.BrokerConfig
	DefaultUploadConfig client.UploadableConfig
	DefaultLogConfig    logger.LogConfig
)

//ResetFlags used in between tests to reset the flags
func ResetFlags() {
	newFlagSet := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	OriginalFlags.VisitAll(func(flag *flag.Flag) {
		newFlagSet.Var(flag.Value, flag.Name, flag.Usage)
	})
	flag.CommandLine = newFlagSet
}

// ToArgs convert ParsedFlags to os args
func (p *ParsedFlags) ToArgs(addDefaults bool, t *testing.T) []*Arg {
	args := configToArgs(p.BrokerConfig, addDefaults, t)
	args = append(args, configToArgs(p.UploadConfig, addDefaults, t)...)
	args = append(args, configToArgs(p.LogConfig, addDefaults, t)...)
	return args
}

// SetTestFullUploadArgs prepare full test args for test
func SetTestFullUploadArgs(config *flags.UploadFileConfig, addDefaults bool, t *testing.T) {
	brokerCfg := &client.BrokerConfig{Broker: config.BrokerConfig.Broker, Username: config.BrokerConfig.Username, Password: config.BrokerConfig.Password}
	activeFromTime := config.UploadableConfig.ActiveFrom
	activeTillTime := config.UploadableConfig.ActiveTill
	stopTimeoutTime := config.UploadableConfig.StopTimeout

	periodDuration := config.UploadableConfig.Period

	uploadCfg := &client.UploadableConfig{Name: config.UploadableConfig.Name, Context: config.UploadableConfig.Context,
		Type:   config.UploadableConfig.Type,
		Period: periodDuration, Active: true, ActiveFrom: activeFromTime, ActiveTill: activeTillTime,
		Delete: true, Checksum: true, SingleUpload: true, StopTimeout: stopTimeoutTime}
	logCfg := &logger.LogConfig{LogFile: config.LogConfig.LogFile, LogLevel: config.LogConfig.LogLevel, LogFileSize: config.LogConfig.LogFileSize,
		LogFileCount: config.LogConfig.LogFileCount, LogFileMaxAge: config.LogConfig.LogFileMaxAge}

	FullUploadParsedFlags = &ParsedFlags{BrokerConfig: brokerCfg, UploadConfig: uploadCfg, LogConfig: logCfg}
	FullUploadParsedFlags.FilesGlob = config.Files
	TestCliFullArgs = FullUploadParsedFlags.ToArgs(addDefaults, t)
	if config.Files != "" || addDefaults {
		TestCliFullArgs = append(TestCliFullArgs, &Arg{flags.Files, config.Files})
	}
}

// PassArgs pass the args to os.Args
func PassArgs(args ...*Arg) {
	var toAppend []string
	for _, e := range args {
		toAppend = append(toAppend, e.String())
	}
	os.Args = append(OriginalArgs, toAppend...)
}

// VerifyEquals compares two interfaces
func VerifyEquals(expected interface{}, actual interface{}, t *testing.T, messageFormat *string) {
	if !reflect.DeepEqual(expected, actual) {
		expectedB, _ := json.Marshal(expected)
		actualB, _ := json.Marshal(actual)
		format := defaultMessageFormat
		if messageFormat != nil && *messageFormat != "" {
			format = *messageFormat
		}
		t.Errorf(format, string(expectedB), string(actualB))
	}
}

// VerifyNotFoundError fails test if file not exist is expected, but not returned and vice versa
func VerifyNotFoundError(returnedError error, expected bool, t *testing.T) {
	if expected {
		if !os.IsNotExist(returnedError) {
			t.Errorf("Expected file not exist error")
		}
	} else {
		if returnedError != nil {
			t.Errorf("Unexpected error %v", returnedError)
		}
	}
}

// RemoveCliArg removes an argument if some test needs to omit it
func RemoveCliArg(name string, args []*Arg) {
	for i, arg := range args {
		if arg.Name == name {
			lastElementIndex := len(args) - 1
			args[i] = args[lastElementIndex]
			args = args[:lastElementIndex]
		}
	}
}

func configToArgs(cfg interface{}, addDefaults bool, t *testing.T) []*Arg {
	var args []*Arg
	if cfg != nil {
		s := reflect.ValueOf(cfg).Elem()
		typeOfS := s.Type()
		for i := 0; i < s.NumField(); i++ {
			f := s.Field(i)
			if addDefaults || f.Interface() != reflect.Zero(f.Type()).Interface() {
				name := typeOfS.Field(i).Name
				rn := []rune(name)
				rn[0] = unicode.ToLower(rn[0])
				args = append(args, &Arg{string(rn), getConfigArgValue(f.Interface(), t)})
			}
		}
	}
	return args
}

func getConfigArgValue(value interface{}, t *testing.T) string {
	switch value.(type) {
	case string:
		return value.(string)
	case int:
		return strconv.Itoa(value.(int))
	case bool:
		return strconv.FormatBool(value.(bool))
	case time.Duration:
		return value.(time.Duration).String()
	case *time.Time:
		return timeToRfc3999(value.(*time.Time))
	case client.Xtime:
		xt := value.(client.Xtime)
		return xt.String()
	case client.Duration:
		dur := value.(client.Duration)
		return dur.String()
	default:
		t.Errorf("Unexpected argument value %v", value)
		return ""
	}
}

func timeToRfc3999(t *time.Time) string {
	if t != nil {
		return t.Format(time.RFC3339)
	}
	return ""
}
