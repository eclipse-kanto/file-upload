// Copyright (c) 2022 Contributors to the Eclipse Foundation
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

package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/eclipse-kanto/file-upload/client"
	mqtt "github.com/eclipse/paho.mqtt.golang"
)

func (suite *uploadSuite) doRequest(method string, url string, params map[string]interface{}) ([]byte, error) {
	var body io.Reader
	var err error
	if params != nil {
		jsonValue, err := json.Marshal(params)
		if err != nil {
			return nil, err
		}
		body = bytes.NewBuffer(jsonValue)
	}
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	if params != nil {
		req.Header.Add("Content-Type", "application/json")
	}

	req.SetBasicAuth(suite.cfg.DittoUser, suite.cfg.DittoPassword)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s %s request failed: %s", method, url, resp.Status)
	}

	return io.ReadAll(resp.Body)
}

func getThingConfig(mqttClient mqtt.Client) (*thingConfig, error) {
	type result struct {
		cfg *thingConfig
		err error
	}

	ch := make(chan result)

	if token := mqttClient.Subscribe("edge/thing/response", 1, func(client mqtt.Client, message mqtt.Message) {
		var cfg thingConfig
		if err := json.Unmarshal(message.Payload(), &cfg); err != nil {
			ch <- result{nil, err}
		}
		ch <- result{&cfg, nil}
	}); token.Wait() && token.Error() != nil {
		return nil, token.Error()
	}

	if token := mqttClient.Publish("edge/thing/request", 1, false, ""); token.Wait() && token.Error() != nil {
		return nil, token.Error()
	}

	timeout := 5 * time.Second
	select {
	case result := <-ch:
		return result.cfg, result.err
	case <-time.After(timeout):
		return nil, fmt.Errorf("thing config not received in %v", timeout)
	}
}

func (suite *uploadSuite) trigger(correlationID string) {
	params := map[string]interface{}{
		correlationID: correlationID,
	}
	suite.execCommand(operationTrigger, params)
}

func (suite *uploadSuite) execCommand(command string, params map[string]interface{}) {
	url := fmt.Sprintf("%s/inbox/messages/%s", suite.featureURL, command)
	suite.doRequest(http.MethodPost, url, params)
}

func (suite *uploadSuite) awaitLastUpload(correlationID string, expectedState string) *lastUpload {
	url := fmt.Sprintf("%s/properties/%s", suite.featureURL, propertyLastUpload)
	for i := 0; i < uploadFilesTimeout; i++ {
		body, err := suite.doRequest(http.MethodGet, url, nil)
		if err == nil && len(body) > 0 {
			lastUpload := &lastUpload{}
			if err := json.Unmarshal(body, lastUpload); err == nil {
				if correlationID == lastUpload.CorrelationID && isTerminal(lastUpload.State) {
					return lastUpload
				}
			}
		}
		time.Sleep(time.Second)
	}
	return nil
}

func isTerminal(state string) bool {
	return state == client.StateSuccess || state == client.StateFailed || state == client.StateCanceled
}
