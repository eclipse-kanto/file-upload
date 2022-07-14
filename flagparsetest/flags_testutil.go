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

	flags "github.com/eclipse-kanto/file-upload/flagparse"
)

// ConfigToArgs convert config to OS command-line args
func ConfigToArgs(t *testing.T, cfg interface{}, skip map[string]bool, addDefaults bool) []Arg {
	a := make([]Arg, 0, 10)

	value := reflect.ValueOf(cfg).Elem()

	configToArgs(t, value, skip, addDefaults, &a)

	return a
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
)

// Common test variables
var (
	OriginalArgs  []string
	originalFlags flag.FlagSet
)

//ResetFlags used in between tests to reset the flags
func ResetFlags() {
	newFlagSet := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	originalFlags.VisitAll(func(flag *flag.Flag) {
		newFlagSet.Var(flag.Value, flag.Name, flag.Usage)
	})
	flag.CommandLine = newFlagSet
}

// PassArgs pass the args to os.Args
func PassArgs(args ...Arg) {
	testArgs := []string{"flagtest"}

	for _, e := range args {
		testArgs = append(testArgs, e.String())
	}
	os.Args = testArgs
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
func RemoveCliArg(name string, args []Arg) []Arg {
	result := make([]Arg, 0, len(args))

	for _, arg := range args {
		if arg.Name != name {
			result = append(result, arg)
		}
	}

	return result
}

func configToArgs(t *testing.T, s reflect.Value, skip map[string]bool, addDefaults bool, args *[]Arg) {
	typeOfS := s.Type()
	for i := 0; i < s.NumField(); i++ {
		field := s.Field(i)
		fieldType := typeOfS.Field(i)
		name := flags.ToFlagName(fieldType.Name)

		if skip != nil && skip[name] {
			continue
		}

		if !fieldType.IsExported() {
			continue
		}

		v, recurse := getConfigArgValue(t, field)

		if recurse {
			configToArgs(t, field, skip, addDefaults, args)
		} else {
			*args = append(*args, Arg{name, v})
		}
	}
}

func getConfigArgValue(t *testing.T, v reflect.Value) (string, bool) {
	flagValue, ok := v.Addr().Interface().(flag.Value)
	if ok {
		return flagValue.String(), false
	}

	if v.Kind() == reflect.Struct {
		return "", true
	}

	value := v.Interface()
	result := ""

	switch value.(type) {
	case string:
		result = value.(string)
	case int:
		result = strconv.Itoa(value.(int))
	case bool:
		result = strconv.FormatBool(value.(bool))
	default:
		t.Errorf("Unexpected argument value %v", value)
	}

	return result, false
}

func timeToRfc3999(t *time.Time) string {
	if t != nil {
		return t.Format(time.RFC3339)
	}
	return ""
}
