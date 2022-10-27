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

package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/eclipse-kanto/file-upload/client"
	flags "github.com/eclipse-kanto/file-upload/flagparse"
	"github.com/eclipse-kanto/file-upload/logger"
)

var version = "dev"

func main() {
	config, warn := flags.ParseFlags(version)

	config.Validate()
	loggerOut, err := logger.SetupLogger(&config.LogConfig, "[FILE UPLOAD]")
	if err != nil {
		log.Fatalln("Failed to initialize logger: ", err)
	}
	defer loggerOut.Close()

	if warn != nil {
		logger.Warn(warn)
	}

	logger.Infof("files glob: '%s', mode: '%s'", config.Files, config.Mode)
	logger.Infof("uploadable config: %+v", config.UploadableConfig)
	logger.Infof("log config: %+v", config.LogConfig)

	if config.Files == "" {
		logger.Debug("No glob filter provided!")
	} else {
		files, err := filepath.Glob(config.Files)
		if err != nil {
			log.Fatalln(err)
		}
		logger.Debugf("Files matching glob filter '%s': %v\n", config.Files, files)
	}

	chstop := make(chan os.Signal, 1)
	signal.Notify(chstop, syscall.SIGINT, syscall.SIGTERM)

	fmt.Println("Press Ctrl+C to exit.")

	uploadable, err := client.NewFileUpload(config.Files, config.Mode, &config.UploadableConfig)
	if err != nil {
		panic(err)
	}

	p, err := client.NewEdgeConnector(&config.BrokerConfig, uploadable)
	if err != nil {
		panic(err)
	}

	defer p.Close()

	<-chstop
}
