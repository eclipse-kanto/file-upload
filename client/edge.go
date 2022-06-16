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

package client

import (
	"encoding/json"
	"time"

	"github.com/eclipse-kanto/file-upload/logger"
	MQTT "github.com/eclipse/paho.mqtt.golang"
	"github.com/google/uuid"
)

const (
	topic = "edge/thing/response"
)

// BrokerConfig contains address and credentials for the MQTT broker
type BrokerConfig struct {
	Broker   string `json:"broker,omitempty" def:"tcp://localhost:1883" descr:"Local MQTT broker address"`
	Username string `json:"username,omitempty" descr:"Username for authorized local client"`
	Password string `json:"password,omitempty" descr:"Password for authorized local client"`
}

// EdgeConfiguration represents local Edge Thing configuration - its device, tenant and policy identifiers.
type EdgeConfiguration struct {
	DeviceID string `json:"deviceId"`
	TenantID string `json:"tenantId"`
	PolicyID string `json:"policyId"`
}

// FetchEdgeConfiguration retrieves local configuration from the Edge Agent
func FetchEdgeConfiguration(cfg *BrokerConfig) (chan *EdgeConfiguration, MQTT.Client, error) {
	logger.Infof("retrieve edge configuration from: %s", cfg.Broker)
	opts := MQTT.NewClientOptions().
		AddBroker(cfg.Broker).
		SetClientID(uuid.New().String()).
		SetKeepAlive(30 * time.Second).
		SetCleanSession(true).
		SetAutoReconnect(true)
	if len(cfg.Username) > 0 {
		opts = opts.SetUsername(cfg.Username).SetPassword(cfg.Password)
	}

	client := MQTT.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		return nil, nil, token.Error()
	}

	cfgChan := make(chan *EdgeConfiguration, 1)

	if token := client.Subscribe(topic, 1, func(client MQTT.Client, message MQTT.Message) {
		localCfg := &EdgeConfiguration{}
		err := json.Unmarshal(message.Payload(), localCfg)
		if err != nil {
			logger.Errorf("could not unmarshal edge configuration: %v", err)
			cfgChan <- nil
			return
		}

		logger.Infof("edge configuration received: %v", localCfg)

		client.Unsubscribe(topic)
		cfgChan <- localCfg
	}); token.Wait() && token.Error() != nil {
		logger.Errorf("error subscribing to %s topic: %v", topic, token.Error())
	}

	if token := client.Publish("edge/thing/request", 1, false, ""); token.Wait() && token.Error() != nil {
		return nil, nil, token.Error()
	}

	return cfgChan, client, nil
}
