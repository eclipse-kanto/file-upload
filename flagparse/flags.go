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

package flags

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"unicode"

	"github.com/eclipse-kanto/file-upload/client"
	"github.com/eclipse-kanto/file-upload/logger"
)

// Flag names and default values
const (
	ConfigFile = "configFile"

	Files = "files"
)

// UploadConfig describes config of uploadable feature
type UploadConfig struct {
	client.BrokerConfig
	client.UploadableConfig
	logger.LogConfig

	Files string            `json:"files,omitempty" descr:"Glob pattern for the files to upload"`
	Mode  client.AccessMode `json:"mode,omitempty" def:"strict" descr:"{mode}"`
}

// ConfigNames contains template names to be replaced in config properties descriptions and default values
var ConfigNames = map[string]string{
	"featureID": "AutoUploadable", "feature": "Uploadable", "period": "Upload period",
	"action": "upload", "actions": "uploads", "running_actions": "uploads",
	"transfers": "uploads", "logFile": "log/file-upload.log",
	"mode": "File access mode. Restricts which files can be requested dynamically for upload through 'upload.files' " +
		"trigger operation property.\nAllowed values are:" +
		"\n  'strict' - dynamically specifying files for upload is forbidden, the 'files' property must be used instead" +
		"\n  'scoped' - allows upload of any files that match the 'files' glob filter" +
		"\n  'lax' - allows upload of any files the upload process has access to",
}

// ConfigFileMissing error, which represents a warning for missing config file
type ConfigFileMissing error

// Validate file upload config
func (cfg *UploadConfig) Validate() {
	if cfg.Files == "" {
		if cfg.Mode != client.ModeLax {
			log.Fatalln("Files glob not specified. To permit unrestricted file upload set 'mode' property to 'lax'.")
		}
	} else {
		_, err := filepath.Glob(cfg.Files)
		if err != nil {
			log.Fatalln(err)
		}
	}
	if (len(cfg.Cert) == 0) != (len(cfg.Key) == 0) {
		log.Fatalln("Either both client MQTT certificate and key must be set or none of them.")
	}
	cfg.UploadableConfig.Validate()
}

// ParseFlags parses the CLI flags and generates an upload file configuration
func ParseFlags(version string) (*UploadConfig, ConfigFileMissing) {

	flagsConfig := &UploadConfig{}
	printVersion := flag.Bool("version", false, "Prints current version and exits")
	configFile := flag.String(ConfigFile, "", "Defines the configuration file")

	InitFlagVars(flagsConfig, ConfigNames, nil)

	flag.Parse()

	if *printVersion {
		fmt.Println(version)
		os.Exit(0)
	}

	config := &UploadConfig{}
	warn := LoadConfigFromFile(*configFile, config, ConfigNames, nil)
	ApplyFlags(config, *flagsConfig)

	return config, warn
}

// ApplyFlags applies CLI values over config values
func ApplyFlags(config interface{}, flagsConfig interface{}) {
	srcVal := reflect.ValueOf(flagsConfig)
	dstVal := reflect.ValueOf(config).Elem()
	flag.Visit(func(f *flag.Flag) {
		name := ToFieldName(f.Name)

		srcField := srcVal.FieldByName(name)
		if srcField.Kind() != reflect.Invalid {
			dstField := dstVal.FieldByName(name)

			dstField.Set(srcField)
		}
	})
}

// InitFlagVars parses the 'cfg' structure and defines flag variables for its fields.
// The 'cfg' parameter should be a pointer to structure. Flag names are taken from field names (with the first letter lower cased).
// The 'names' parameter should be used for generating a strings.Replacer, replacing the keys(surrounded with {}) with their values.
// The 'skip' parameter lists the flag names, that should not be parsed from configuration.
// Flag descriptions are taken from 'descr' field tags, default values are taken from 'def' field tags
func InitFlagVars(cfg interface{}, names map[string]string, skip map[string]bool) {
	initConfigValues(reflect.ValueOf(cfg).Elem(), names, skip, true)
}

// LoadConfigFromFile loads the config from the specified file into the given config structure.
// The 'cfg' parameter should be a pointer to structure
// The 'names' parameter should be used for generating a strings.Replacer, replacing the keys(surrounded with {}) with their values.
// The 'skip' parameter lists the flag names, that should not be parsed from configuration.
func LoadConfigFromFile(configFile string, cfg interface{}, names map[string]string, skip map[string]bool) ConfigFileMissing {
	initConfigValues(reflect.ValueOf(cfg).Elem(), names, skip, false)

	var warn ConfigFileMissing

	// Load configuration file (if possible)
	if len(configFile) > 0 {
		err := LoadJSON(configFile, cfg)

		if err != nil {
			if os.IsNotExist(err) {
				warn = err
			} else {
				log.Fatalf("Error reading config file: %v", err)
			}
		}
	}

	return warn
}

func initConfigValues(valueOfConfig reflect.Value, names map[string]string, skip map[string]bool, flagIt bool) {
	r := getReplacer(names)

	typeOfConfig := valueOfConfig.Type()
	numFields := typeOfConfig.NumField()
	for i := 0; i < numFields; i++ {
		fieldType := typeOfConfig.Field(i)
		argName := ToFlagName(fieldType.Name)

		if skip != nil && skip[argName] {
			continue
		}

		if !fieldType.IsExported() {
			continue
		}

		defaultValue := fieldType.Tag.Get("def")
		description := fieldType.Tag.Get("descr")

		if r != nil {
			defaultValue = r.Replace(defaultValue)
			description = r.Replace(description)
		}

		fieldValue := valueOfConfig.FieldByName(fieldType.Name)
		pointer := fieldValue.Addr().Interface()

		switch val := fieldValue.Interface(); val.(type) {
		case string:
			if flagIt {
				flag.StringVar(pointer.(*string), argName, defaultValue, description)
			} else {
				fieldValue.SetString(defaultValue)
			}
		case bool:
			defaultBoolValue, err := strconv.ParseBool(defaultValue)
			if err != nil {
				log.Printf("Error parsing boolean argument %v with value %v", fieldType.Name, defaultValue)
			}
			if flagIt {
				flag.BoolVar(pointer.(*bool), argName, defaultBoolValue, description)
			} else {
				fieldValue.SetBool(defaultBoolValue)
			}
		case int:
			defaultIntValue, err := strconv.Atoi(defaultValue)
			if err != nil {
				log.Printf("Error parsing integer argument %v with value %v", fieldType.Name, defaultValue)
			}
			if flagIt {
				flag.IntVar(pointer.(*int), argName, defaultIntValue, description)
			} else {
				fieldValue.SetInt(int64(defaultIntValue))
			}
		default:
			v, ok := pointer.(flag.Value)

			if ok {
				if flagIt {
					flag.Var(v, argName, description)
				} else if err := v.Set(defaultValue); err == nil {
					fieldValue.Set(reflect.ValueOf(v).Elem())
				} else {
					log.Printf("Error parsing argument %v with value %v - %v", fieldType.Name, defaultValue, err)
				}
			} else if fieldType.Type.Kind() == reflect.Struct {
				initConfigValues(fieldValue, names, skip, flagIt)
			}
		}
	}
}

func getReplacer(names map[string]string) *strings.Replacer {
	if names == nil {
		return nil
	}
	oldNew := make([]string, 0, len(names)*2)
	for k, v := range names {
		oldNew = append(oldNew, "{"+k+"}")
		oldNew = append(oldNew, v)
	}

	return strings.NewReplacer(oldNew...)
}

// InitConfigDefaults sets the default field values of the passed config.
// The 'cfg' should be a pointer to config. Default values are extracted from 'def' field tags
func InitConfigDefaults(cfg interface{}, mapping map[string]string, skip map[string]bool) {
	initConfigValues(reflect.ValueOf(cfg).Elem(), mapping, skip, false)
}

// LoadJSON loads a json file from path into a given interface
func LoadJSON(file string, v interface{}) error {
	b, err := ioutil.ReadFile(file)
	if err == nil {
		err = json.Unmarshal(b, v)
	}

	return err
}

// ToFieldName converts command-line flag name to config structure field name
func ToFieldName(s string) string {
	s = replaceSuffix(s, "Id", "ID")
	rn := []rune(s)
	rn[0] = unicode.ToUpper(rn[0])
	return string(rn)
}

// ToFlagName converts config structure field name to command-line flag name
func ToFlagName(s string) string {
	s = replaceSuffix(s, "ID", "Id")
	rn := []rune(s)
	rn[0] = unicode.ToLower(rn[0])
	return string(rn)
}

// replaceSuffix replaces a suffix of a string with another suffix
func replaceSuffix(s, suff, replacement string) string {
	if strings.HasSuffix(s, suff) {
		return s[:len(s)-len(suff)] + replacement
	}
	return s
}
