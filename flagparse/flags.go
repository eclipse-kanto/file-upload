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

//UploadConfig describes config of uploadable feature
type UploadConfig struct {
	client.BrokerConfig
	client.UploadableConfig
	logger.LogConfig

	Files string `json:"files,omitempty" descr:"Glob pattern for the files to upload"`
}

//ConfigNames contains template names to be replaced in config properties descriptions and default values
var ConfigNames = map[string]string{
	"name": "Autouploadable", "feature": "Uploadable", "period": "Upload period",
	"action": "upload", "actions": "uploads", "running_actions": "uploads",
}

//ConfigFileMissing error, which represents a warning for missing config file
type ConfigFileMissing error

//ParseFlags parses the CLI flags and generates an upload file configuration
func ParseFlags(version string) (*UploadConfig, ConfigFileMissing) {
	dumpFiles := flag.Bool("dumpFiles", false, "On startup dump the file paths matching the '-files' glob pattern to standard output.")

	flagsConfig := &UploadConfig{}
	printVersion := flag.Bool("version", false, "Prints current version and exits")
	configFile := flag.String(ConfigFile, "", "Defines the configuration file")

	initFlagVars(flagsConfig, ConfigNames, nil)

	flag.Parse()

	if *printVersion {
		fmt.Println(version)
		os.Exit(0)
	}

	config := &UploadConfig{}
	warn := loadConfigFromFile(*configFile, config, ConfigNames, nil)
	applyFlags(config, *flagsConfig)

	if *dumpFiles {
		if config.Files == "" {
			fmt.Println("No glob filter provided!")
		} else {
			files, err := filepath.Glob(config.Files)
			if err != nil {
				log.Fatalln(err)
			}
			fmt.Printf("Files matching glob filter '%s': %v\n", config.Files, files)
		}
	}

	return config, warn
}

// applyFlags applies CLI values over config values
func applyFlags(config interface{}, flagsConfig interface{}) {
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

//initFlagVars parses the 'cfg' structure and defines flag variables for its fields.
//The 'cfg' parameter should be a pointer to structure. Flag names are taken from field names (with the first letter lower cased).
//The 'names' parameter should be used for generating a strings.Replacer, replacing the keys(surrounded with {}) with their values.
//The 'skip' parameter lists the flag names, that should not be parsed from configuration.
//Flag descriptions are taken from 'descr' field tags, default values are taken from 'def' field tags
func initFlagVars(cfg interface{}, names map[string]string, skip map[string]bool) {
	initConfigValues(reflect.ValueOf(cfg).Elem(), names, skip, true)
}

//loadConfigFromFile loads the config from the specified file into the given config structure.
//The 'cfg' parameter should be a pointer to structure
//The 'names' parameter should be used for generating a strings.Replacer, replacing the keys(surrounded with {}) with their values.
//The 'skip' parameter lists the flag names, that should not be parsed from configuration.
func loadConfigFromFile(configFile string, cfg interface{}, names map[string]string, skip map[string]bool) ConfigFileMissing {
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

//InitConfigDefaults sets the default field values of the passed config.
//The 'cfg' should be a pointer to config. Default values are extracted from 'def' field tags
func InitConfigDefaults(cfg interface{}, mapping map[string]string, skip map[string]bool) {
	initConfigValues(reflect.ValueOf(cfg).Elem(), mapping, skip, false)
}

//LoadJSON loads a json file from path into a given interface
func LoadJSON(file string, v interface{}) error {
	b, err := ioutil.ReadFile(file)
	if err == nil {
		err = json.Unmarshal(b, v)
	}

	return err
}

//ToFlagName converts config structure field name to command-line flag name
func ToFlagName(s string) string {
	rn := []rune(s)
	rn[0] = unicode.ToLower(rn[0])
	return string(rn)
}

//ToFieldName converts command-line flag name to config structure field name
func ToFieldName(s string) string {
	rn := []rune(s)
	rn[0] = unicode.ToUpper(rn[0])
	return string(rn)
}
