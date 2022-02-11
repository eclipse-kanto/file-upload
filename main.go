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

package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/eclipse-kanto/file-upload/client"
	flags "github.com/eclipse-kanto/file-upload/flagparse"
	"github.com/eclipse-kanto/file-upload/logger"
)

var version = "dev"

func main() {
	brokerCfg, uploadCfg, logCfg, filesGlob, warn := flags.ParseFlags(version)

	uploadCfg.Validate()
	loggerOut, err := logger.SetupLogger(logCfg)
	if err != nil {
		log.Fatalln("Failed to initialize logger: ", err)
	}
	defer loggerOut.Close()

	if warn != nil {
		logger.Warning(warn)
	}

	logger.Infof("uploadable config: %+v", uploadCfg)
	logger.Infof("log config: %+v", logCfg)

	chCfg, broker, err := client.FetchEdgeConfiguration(brokerCfg)
	if err != nil {
		panic(err)
	}

	chstop := make(chan os.Signal, 1)
	signal.Notify(chstop, syscall.SIGINT, syscall.SIGTERM)

	fmt.Println("Press Ctrl+C to exit.")

	var edgeCfg *client.EdgeConfiguration

	select {
	case <-chstop:
		broker.Disconnect(200)
		return
	case cfg := <-chCfg:
		edgeCfg = cfg
	}

	uploadable, err := client.NewFileUpload(filesGlob, broker, edgeCfg, uploadCfg)
	if err != nil {
		panic(err)
	}

	if err := uploadable.Connect(); err != nil {
		panic(err)
	}

	defer func() {
		uploadable.Disconnect()
		logger.Info("disconnected from MQTT broker")
	}()

	<-chstop
}
