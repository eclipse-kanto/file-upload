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
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/websocket"
)

func (suite *testSuite) doRequest(method string, url string, contentType string, params map[string]interface{}) ([]byte, error) {
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
	if len(contentType) > 0 {
		req.Header.Add("Content-Type", contentType)
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

func (suite *testSuite) execCommand(command string, params map[string]interface{}) {
	url := fmt.Sprintf("%s/inbox/messages/%s", suite.featureURL, command)
	suite.doRequest(http.MethodPost, url, "application/json", params)
}

func (suite *testSuite) startEventListener(eventType string, matcher func(map[string]interface{}) bool) (*websocket.Conn, chan bool) {
	ws, err := suite.newWSConnection()
	require.NoError(suite.T(), err)

	subAck := fmt.Sprintf("%s:ACK", eventType)
	var ackReceived bool
	ackChan := make(chan bool)
	wsListener := func(payload []byte) bool {
		ack := strings.TrimSpace(string(payload))
		if ack == subAck {
			ackReceived = true
			ackChan <- true
			return false
		}
		if !ackReceived {
			suite.T().Logf("skipping event, acknowledgement not received")
			return false
		}
		props := make(map[string]interface{})
		err := json.Unmarshal(payload, &props)
		if err == nil {
			return matcher(props)
		}

		suite.T().Logf("error while waiting for event: %v", err)
		return false
	}
	websocket.Message.Send(ws, fmt.Sprintf("%s?filter=like(resource:path,'/features/%s/*')", eventType, featureID))
	result := suite.beginWSWait(ws, wsListener)
	require.True(suite.T(), suite.awaitChan(ackChan), "event acknowledgement not received")
	return ws, result
}

func (suite *testSuite) beginWSWait(ws *websocket.Conn, check func(payload []byte) bool) chan bool {
	timeout := time.Duration(suite.cfg.EventTimeoutMs * int(time.Millisecond))

	ch := make(chan bool)

	go func() {
		resultCh := make(chan bool)

		go func() {
			var payload []byte
			threshold := time.Now().Add(timeout)
			for time.Now().Before(threshold) {
				err := websocket.Message.Receive(ws, &payload)
				if err == nil {
					if check(payload) {
						resultCh <- true
						return
					}
				} else {
					suite.T().Logf("error while waiting for WS message: %v", err)
				}
			}

			suite.T().Logf("WS response not received in %v", timeout)

			resultCh <- false
		}()
		result := suite.awaitChan(resultCh)
		ws.Close()
		ch <- result
	}()

	return ch
}

func (suite *testSuite) newWSConnection() (*websocket.Conn, error) {
	wsAddress, err := asWSAddress(suite.cfg.DittoAddress)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/ws/2", wsAddress)
	cfg, err := websocket.NewConfig(url, suite.cfg.DittoAddress)
	if err != nil {
		return nil, err
	}

	auth := fmt.Sprintf("%s:%s", suite.cfg.DittoUser, suite.cfg.DittoPassword)
	enc := base64.StdEncoding.EncodeToString([]byte(auth))
	cfg.Header = http.Header{
		"Authorization": {"Basic " + enc},
	}

	return websocket.DialConfig(cfg)
}

func asWSAddress(address string) (string, error) {
	url, err := url.Parse(address)
	if err != nil {
		return "", err
	}

	if url.Scheme == "https" {
		return fmt.Sprintf("wss://%s:%s", url.Hostname(), url.Port()), nil
	}

	return fmt.Sprintf("ws://%s:%s", url.Hostname(), url.Port()), nil
}

func (suite *testSuite) awaitChan(ch chan bool) bool {
	timeout := time.Duration(suite.cfg.EventTimeoutMs * int(time.Millisecond))
	select {
	case result := <-ch:
		return result
	case <-time.After(timeout):
		return false
	}
}
