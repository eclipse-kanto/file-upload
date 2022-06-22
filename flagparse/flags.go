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
	"time"
	"unicode"

	"github.com/eclipse-kanto/file-upload/client"
	"github.com/eclipse-kanto/file-upload/logger"
)

// Flag names and default values
const (
	ConfigFile = "configFile"

	Files        = "files"
	DefaultFiles = ""
)

//UploadFileConfig describes file config of uploadable
type UploadFileConfig struct {
	*client.BrokerConfig
	*client.UploadableConfig
	*logger.LogConfig

	Files string `json:"files,omitempty"`
}

//ConfigFileMissing error, which represents a warning for missing config file
type ConfigFileMissing error

//NewUploadFileConfig return initialized UploadFileConfig
func NewUploadFileConfig() *UploadFileConfig {
	return &UploadFileConfig{
		&client.BrokerConfig{},
		&client.UploadableConfig{},
		&logger.LogConfig{},
		"",
	}
}

//ParseFlags Define & Parse all flags
func ParseFlags(version string) (*client.BrokerConfig, *client.UploadableConfig, *logger.LogConfig, string, ConfigFileMissing) {
	dumpFiles := flag.Bool("dumpFiles", false, "On startup dump the file paths matching the '-files' glob pattern to standard output.")

	config := NewUploadFileConfig()
	printVersion := flag.Bool("version", false, "Prints current version and exits")
	configFile := flag.String(ConfigFile, "", "Defines the configuration file")

	brokerConfig := &client.BrokerConfig{}
	uploadConfig := &client.UploadableConfig{}
	logConfig := &logger.LogConfig{}
	filesGlob := ""

	FlagBroker(config)
	FlagUploadable(config)
	FlagLogger(config)
	flag.StringVar(&config.Files, Files, DefaultFiles, "Glob pattern for the files to upload")

	flag.Parse()

	if *printVersion {
		fmt.Println(version)
		os.Exit(0)
	}

	warn := applyConfigurationFile(*configFile, brokerConfig, uploadConfig, logConfig, &filesGlob)
	ApplyFlags(config, brokerConfig, uploadConfig, logConfig, &filesGlob)
	if filesGlob == "" {
		log.Fatalln("Use '-files' command flag to specify glob pattern for the files to upload!")
	}

	if *dumpFiles {
		files, err := filepath.Glob(filesGlob)
		if err != nil {
			log.Fatalln(err)
		}
		fmt.Printf("Files matching glob filter '%s': %v\n", filesGlob, files)
	}

	return brokerConfig, uploadConfig, logConfig, filesGlob, warn
}

// ApplyFlags applies cli values over config values
func ApplyFlags(config *UploadFileConfig, brokerConfig *client.BrokerConfig,
	uploadConfig *client.UploadableConfig, logConfig *logger.LogConfig, filesGlob *string) {

	srcBroker := reflect.ValueOf(config.BrokerConfig).Elem()
	valBroker := reflect.ValueOf(brokerConfig).Elem()

	srcUpload := reflect.ValueOf(config.UploadableConfig).Elem()
	valUpload := reflect.ValueOf(uploadConfig).Elem()

	srcLog := reflect.ValueOf(config.LogConfig).Elem()
	valLog := reflect.ValueOf(logConfig).Elem()

	flag.Visit(func(f *flag.Flag) {
		upperCaseName := toUpper(f.Name)
		switch {
		case copyFieldIfExists(upperCaseName, srcBroker, valBroker): // copying done, if check is ok, otherwise continue
		case copyFieldIfExists(upperCaseName, srcUpload, valUpload): // copying done, if check is ok, otherwise continue
		case copyFieldIfExists(upperCaseName, srcLog, valLog): // copying done, if check is ok, otherwise continue
		case f.Name == Files:
			*filesGlob = config.Files
		default:
			// Unknown flag
		}
	})
}

func applyConfigurationFile(configFile string, brokerConfig *client.BrokerConfig,
	uploadConfig *client.UploadableConfig, logConfig *logger.LogConfig, filesGlob *string) ConfigFileMissing {
	def := GetUploadFileConfigDefaults()
	var warn ConfigFileMissing

	// Load configuration file (if possible)
	if len(configFile) > 0 {
		err := LoadJSON(configFile, def)

		if err != nil {
			if os.IsNotExist(err) {
				warn = err
			} else {
				log.Fatalf("Error reading config file: %v", err)
			}
		}
	}
	SetBrokerConfig(def, brokerConfig)
	SetUploadableConfig(def, uploadConfig)
	SetLoggerConfig(def, logConfig)
	*filesGlob = def.Files

	return warn
}

func initConfigValues(typeOfConfig reflect.Type, valueOfConfig reflect.Value, flagIt bool) {
	for i := 0; i < typeOfConfig.NumField(); i++ {
		fieldType := typeOfConfig.Field(i)
		defaultValue := fieldType.Tag.Get("def")
		description := fieldType.Tag.Get("descr")
		fieldValue := valueOfConfig.FieldByName(fieldType.Name)
		pointer := fieldValue.Addr().Interface()
		argName := toLower(fieldType.Name)
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
				log.Println(fmt.Sprintf("Error parsing boolean argument %v with value %v", fieldType.Name, defaultValue))
			}
			if flagIt {
				flag.BoolVar(pointer.(*bool), argName, defaultBoolValue, description)
			} else {
				fieldValue.SetBool(defaultBoolValue)
			}
		case int:
			defaultIntValue, err := strconv.Atoi(defaultValue)
			if err != nil {
				log.Println(fmt.Sprintf("Error parsing integer argument %v with value %v", fieldType.Name, defaultValue))
			}
			if flagIt {
				flag.IntVar(pointer.(*int), argName, defaultIntValue, description)
			} else {
				fieldValue.SetInt(int64(defaultIntValue))
			}
		default:
			if flagIt {
				flag.Var(pointer.(flag.Value), argName, description)
			} else {
				if fieldType.Type.Name() == "Duration" {
					duration, err := time.ParseDuration(defaultValue)
					if err != nil {
						log.Println(fmt.Sprintf("Error parsing duration argument %v with value %v", fieldType.Name, defaultValue))
					}
					fieldValue.Set(reflect.ValueOf(client.Duration(duration)))
				}
			}
		}
	}
}

//FlagUploadable flags uploadable
func FlagUploadable(config *UploadFileConfig) {
	initConfigValues(reflect.TypeOf(*config.UploadableConfig), reflect.ValueOf(config.UploadableConfig).Elem(), true)
}

//FlagBroker flags broker
func FlagBroker(config *UploadFileConfig) {
	initConfigValues(reflect.TypeOf(*config.BrokerConfig), reflect.ValueOf(config.BrokerConfig).Elem(), true)
}

//FlagLogger flags logger
func FlagLogger(config *UploadFileConfig) {
	initConfigValues(reflect.TypeOf(*config.LogConfig), reflect.ValueOf(config.LogConfig).Elem(), true)
}

//GetUploadFileConfigDefaults returns new *UploadFileConfig with default values set.
func GetUploadFileConfigDefaults() *UploadFileConfig {
	brokerConfig := client.BrokerConfig{}
	uploadConfig := client.UploadableConfig{}
	logConfig := logger.LogConfig{}
	initConfigValues(reflect.TypeOf(brokerConfig), reflect.ValueOf(&brokerConfig).Elem(), false)
	initConfigValues(reflect.TypeOf(uploadConfig), reflect.ValueOf(&uploadConfig).Elem(), false)
	initConfigValues(reflect.TypeOf(logConfig), reflect.ValueOf(&logConfig).Elem(), false)
	return &UploadFileConfig{&brokerConfig, &uploadConfig, &logConfig, DefaultFiles}
}

//SetBrokerConfig sets BrokerConfig values from file config
func SetBrokerConfig(def *UploadFileConfig, brokerConfig *client.BrokerConfig) {
	copyConfigData(def.BrokerConfig, brokerConfig)
}

//SetUploadableConfig sets UploadableConfig from file config
func SetUploadableConfig(def *UploadFileConfig, uploadConfig *client.UploadableConfig) {
	copyConfigData(def.UploadableConfig, uploadConfig)
}

//SetLoggerConfig sets LogConfig from file config
func SetLoggerConfig(def *UploadFileConfig, logConfig *logger.LogConfig) {
	copyConfigData(def.LogConfig, logConfig)
}

//LoadJSON loads a json file from path into a given interface
func LoadJSON(file string, v interface{}) error {
	b, err := ioutil.ReadFile(file)
	if err == nil {
		err = json.Unmarshal(b, v)
	}

	return err
}

func copyConfigData(sourceConfig interface{}, targetConfig interface{}) {
	source := reflect.ValueOf(sourceConfig).Elem()
	target := reflect.ValueOf(targetConfig).Elem()
	typeOfSource := source.Type()
	for i := 0; i < source.NumField(); i++ {
		fieldTarget := target.FieldByName(typeOfSource.Field(i).Name)
		fieldTarget.Set(reflect.ValueOf(source.Field(i).Interface()))
	}
}

func toLower(s string) string {
	rn := []rune(s)
	rn[0] = unicode.ToLower(rn[0])
	return string(rn)
}

func toUpper(s string) string {
	rn := []rune(s)
	rn[0] = unicode.ToUpper(rn[0])
	return string(rn)
}

func copyFieldIfExists(name string, source, target reflect.Value) bool {
	if field := target.FieldByName(name); field != reflect.Zero(reflect.TypeOf(field)).Interface() {
		field.Set(reflect.ValueOf(source.FieldByName(name).Interface()))
		return true
	}
	return false
}
