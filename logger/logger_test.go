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

package logger

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLogLevelError tests logger functions with log level set to ERROR.
func TestLogLevelError(t *testing.T) {
	validate("ERROR", true, false, false, false, false, t)
}

// TestLogLevelWarning tests logger functions with log level set to WARNING.
func TestLogLevelWarning(t *testing.T) {
	validate("WARNING", true, true, false, false, false, t)
}

// TestLogLevelInfo tests logger functions with log level set to INFO.
func TestLogLevelInfo(t *testing.T) {
	validate("INFO", true, true, true, false, false, t)
}

// TestLogLevelDebug tests logger functions with log level set to DEBUG.
func TestLogLevelDebug(t *testing.T) {
	validate("DEBUG", true, true, true, true, false, t)
}

// TestLogLevelTrace tests logger functions with log level set to TRACE.
func TestLogLevelTrace(t *testing.T) {
	validate("TRACE", true, true, true, true, true, t)
}

// TestNopWriter tests logger functions without writter.
func TestNopWriter(t *testing.T) {
	// Prepare
	dir := "_tmp-logger"
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("failed create temporary directory: %v", err)
	}
	defer os.RemoveAll(dir)

	// Prepare the logger without writter
	loggerOut, _ := SetupLogger(&LogConfig{LogFile: "", LogLevel: "TRACE", LogFileSize: 2, LogFileCount: 5})
	defer loggerOut.Close()

	// Validate that temporary is empty
	Error("test error")
	f, err := os.Open(dir)
	if err != nil {
		t.Fatalf("cannot open temporary directory: %v", err)
	}
	defer f.Close()

	if _, err = f.Readdirnames(1); err != io.EOF {
		t.Errorf("temporary directory is not empty")
	}
}

func validate(lvl string, hasError bool, hasWarning bool, hasInfo bool, hasDebug bool, hasTrace bool, t *testing.T) {
	// Prepare
	dir := "_tmp-logger"
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("failed create temporary directory: %v", err)
	}
	defer os.RemoveAll(dir)

	// Prepare the logger
	log := filepath.Join(dir, lvl+".log")
	loggerOut, err := SetupLogger(&LogConfig{LogFile: log, LogLevel: lvl, LogFileSize: 2, LogFileCount: 5})
	if err != nil {
		t.Fatal(err)
	}
	defer loggerOut.Close()

	// 1. Validate for error logs.
	validateError(log, hasError, t)

	// 2. Validate for warning logs.
	validateWarning(log, hasWarning, t)

	// 3. Validate for info logs.
	validateInfo(log, hasInfo, t)

	// 4. Validate for debug logs.
	validateDebug(log, hasDebug, t)

	// 5. Validate for trace logs.
	validateTrace(log, hasTrace, t)
}

// validateError validates for error logs.
func validateError(log string, has bool, t *testing.T) {
	// 1. Validate for Error function.
	Error("error log")
	if has != search(log, t, ePrefix, "error log") {
		t.Errorf("error entry mishmash [result: %v]", !has)
	}
	// 2. Validate for Errorf function.
	Errorf("error log [%v,%s]", "param1", "param2")
	if has != search(log, t, ePrefix, "error log [param1,param2]") {
		t.Errorf("errorf entry mishmash: [result: %v]", !has)
	}
}

// validateError validates for warning logs.
func validateWarning(log string, has bool, t *testing.T) {
	// 1. Validate for Warning function.
	Warning("warning log")
	if has != search(log, t, wPrefix, "warning log") {
		t.Errorf("warning entry mishmash [result: %v]", !has)
	}
	// 2. Validate for Warningf function.
	Warningf("warning log [%v,%s]", "param1", "param2")
	if has != search(log, t, wPrefix, "warning log [param1,param2]") {
		t.Errorf("warningf entry mishmash: [result: %v]", !has)
	}
}

// validateError validates for info logs.
func validateInfo(log string, has bool, t *testing.T) {
	// 1. Validate for Info function.
	Info("info log")
	if has != search(log, t, iPrefix, "info log") {
		t.Errorf("info entry mishmash [result: %v]", !has)
	}
	// 2. Validate for Infof function.
	Infof("info log [%v,%s]", "param1", "param2")
	if has != search(log, t, iPrefix, "info log [param1,param2]") {
		t.Errorf("infof entry mishmash: [result: %v]", !has)
	}
}

// validateError validates for debug logs.
func validateDebug(log string, has bool, t *testing.T) {
	// 1. Validate for Debug function.
	Debug("debug log")
	if has != search(log, t, dPrefix, "debug log") {
		t.Errorf("debug entry mishmash [result: %v]", !has)
	}
	// 2. Validate for Debugf function.
	Debugf("debug log [%v,%s]", "param1", "param2")
	if has != search(log, t, dPrefix, "debug log [param1,param2]") {
		t.Errorf("debugf entry mishmash: [result: %v]", !has)
	}
}

// validateError validates for trace logs.
func validateTrace(log string, has bool, t *testing.T) {
	// 1. Validate for Trace function.
	Trace("trace log")
	if has != search(log, t, tPrefix, "trace log") {
		t.Errorf("trace entry mishmash [result: %v]", !has)
	}
	// 2. Validate for Tracef function.
	Tracef("trace log [%v,%s]", "param1", "param2")
	if has != search(log, t, tPrefix, "trace log [param1,param2]") {
		t.Errorf("tracef entry mishmash: [result: %v]", !has)
	}
}

// search strings in log file.
func search(fn string, t *testing.T, entries ...string) bool {
	file, err := os.Open(fn)
	if err != nil {
		t.Fatalf("fail to open log file: %v", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if has(scanner.Text(), entries...) {
			return true
		}
	}

	if err := scanner.Err(); err != nil {
		t.Fatalf("fail to read log file: %v", err)
	}
	return false
}

// has checks if string has substrings
func has(s string, substrs ...string) bool {
	for _, substr := range substrs {
		if !strings.Contains(s, substr) {
			return false
		}
	}
	return true
}
