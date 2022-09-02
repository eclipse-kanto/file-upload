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
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/natefinch/lumberjack.v2"
)

// LogConfig contains logging configuration
type LogConfig struct {
	LogFile       string `json:"logFile,omitempty" def:"{logFile}" descr:"Log file location in storage directory"`
	LogLevel      string `json:"logLevel,omitempty" def:"INFO" descr:"Log levels are ERROR, WARN, INFO, DEBUG, TRACE"`
	LogFileSize   int    `json:"logFileSize,omitempty" def:"2" descr:"Log file size in MB before it gets rotated"`
	LogFileCount  int    `json:"logFileCount,omitempty" def:"5" descr:"Log file max rotations count"`
	LogFileMaxAge int    `json:"logFileMaxAge,omitempty" def:"28" descr:"Log file rotations max age in days"`
}

// LogLevel - Error(1), Warn(2), Info(3), Debug(4) or Trace(5)
type LogLevel int

// Constants for log level
const (
	ERROR LogLevel = 1 + iota
	WARN
	INFO
	DEBUG
	TRACE
)

const (
	logFlags int = log.Ldate | log.Ltime | log.Lmicroseconds | log.Lmsgprefix

	ePrefix = "ERROR  "
	wPrefix = "WARN   "
	iPrefix = "INFO   "
	dPrefix = "DEBUG  "
	tPrefix = "TRACE  "

	prefix = " %-10s"
)

var (
	logger *log.Logger
	level  LogLevel
)

// SetupLogger initializes logger with the provided configuration
func SetupLogger(logConfig *LogConfig, componentPrefix string) (io.WriteCloser, error) {
	loggerOut := io.WriteCloser(&nopWriterCloser{out: os.Stderr})
	if len(logConfig.LogFile) > 0 {
		err := os.MkdirAll(filepath.Dir(logConfig.LogFile), 0755)

		if err != nil {
			return nil, err
		}

		loggerOut = &lumberjack.Logger{
			Filename:   logConfig.LogFile,
			MaxSize:    logConfig.LogFileSize,
			MaxBackups: logConfig.LogFileCount,
			MaxAge:     logConfig.LogFileMaxAge,
			LocalTime:  true,
			Compress:   true,
		}
	}

	log.SetOutput(loggerOut)
	log.SetFlags(logFlags)

	logger = log.New(loggerOut, fmt.Sprintf(prefix, componentPrefix), logFlags)

	// Parse log level
	switch strings.ToUpper(logConfig.LogLevel) {
	case "INFO":
		level = INFO
	case "WARN":
		level = WARN
	case "DEBUG":
		level = DEBUG
	case "TRACE":
		level = TRACE
	default:
		level = ERROR
	}

	return loggerOut, nil
}

// Error logs the given value, if level is >= ERROR
func Error(v interface{}) {
	if level >= ERROR {
		logger.Println(ePrefix, v)
	}
}

// Errorf logs the given formatted message, if level is >= ERROR
func Errorf(format string, v ...interface{}) {
	if level >= ERROR {
		logger.Println(fmt.Errorf(fmt.Sprint(ePrefix, " ", format), v...))
	}
}

// Warn logs the given value, if level is >= WARN
func Warn(v interface{}) {
	if level >= WARN {
		logger.Println(wPrefix, v)
	}
}

// Warnf logs the given formatted message, if level is >= WARN
func Warnf(format string, v ...interface{}) {
	if level >= WARN {
		logger.Printf(fmt.Sprint(wPrefix, " ", format), v...)
	}
}

// Info logs the given value, if level is >= INFO
func Info(v interface{}) {
	if level >= INFO {
		logger.Println(iPrefix, v)
	}
}

// Infof logs the given formatted message, if level is >= INFO
func Infof(format string, v ...interface{}) {
	if level >= INFO {
		logger.Printf(fmt.Sprint(iPrefix, " ", format), v...)
	}
}

// Debug logs the given value, if level is >= DEBUG
func Debug(v interface{}) {
	if IsDebugEnabled() {
		logger.Println(dPrefix, v)
	}
}

// Debugf logs the given formatted message, if level is >= DEBUG
func Debugf(format string, v ...interface{}) {
	if IsDebugEnabled() {
		logger.Printf(fmt.Sprint(dPrefix, " ", format), v...)
	}
}

// Trace logs the given value, if level is >= TRACE
func Trace(v ...interface{}) {
	if IsTraceEnabled() {
		logger.Println(tPrefix, fmt.Sprint(v...))
	}
}

// Tracef logs the given formatted message, if level is >= TRACE
func Tracef(format string, v ...interface{}) {
	if IsTraceEnabled() {
		logger.Printf(fmt.Sprint(tPrefix, " ", format), v...)
	}
}

// IsDebugEnabled returns true if log level is above DEBUG
func IsDebugEnabled() bool {
	return level >= DEBUG
}

// IsTraceEnabled returns true if log level is above TRACE
func IsTraceEnabled() bool {
	return level >= TRACE
}

type nopWriterCloser struct {
	out io.Writer
}

// Write to log output
func (w *nopWriterCloser) Write(p []byte) (n int, err error) {
	return w.out.Write(p)
}

// Close does nothing
func (*nopWriterCloser) Close() error {
	return nil
}
