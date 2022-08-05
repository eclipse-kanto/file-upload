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
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io/ioutil"
	"time"

	"github.com/eclipse-kanto/file-upload/logger"
	"github.com/eclipse-kanto/file-upload/uploaders"
	MQTT "github.com/eclipse/paho.mqtt.golang"
	"github.com/google/uuid"
)

const (
	topic = "edge/thing/response"
)

// BrokerConfig contains address and credentials for the MQTT broker
type BrokerConfig struct {
	Broker     string `json:"broker,omitempty" def:"tcp://localhost:1883" descr:"Local MQTT broker address"`
	Username   string `json:"username,omitempty" descr:"Username for authorized local client"`
	Password   string `json:"password,omitempty" descr:"Password for authorized local client"`
	CaCert     string `json:"caCert,omitempty" descr:"A PEM encoded certificate authority, used to sign the certificate of the local MQTT broker"`
	DeviceCert string `json:"deviceCert,omitempty" descr:"A PEM encoded client certificate for connection to local MQTT broker"`
	DeviceKey  string `json:"deviceKey,omitempty" descr:"А private key for the client certificate, specified with 'deviceCert'"`
}

// EdgeConfiguration represents local Edge Thing configuration - its device, tenant and policy identifiers.
type EdgeConfiguration struct {
	DeviceID string `json:"deviceId"`
	TenantID string `json:"tenantId"`
	PolicyID string `json:"policyId"`
}

// EdgeConnector listens for Edge Thing configuration changes and notifies the corresponding EdgeClient
type EdgeConnector struct {
	mqttClient MQTT.Client
	cfg        *EdgeConfiguration
	edgeClient EdgeClient
}

// EdgeClient receives notifications of Edge Thing configuration changes from EdgeConnector
type EdgeClient interface {
	Connect(client MQTT.Client, cfg *EdgeConfiguration)
	Disconnect()
}

// NewEdgeConnector create EdgeConnector with the given BrokerConfig for the given EdgeClient
func NewEdgeConnector(cfg *BrokerConfig, ecl EdgeClient) (*EdgeConnector, error) {
	var certificates []tls.Certificate
	var caCertPool *x509.CertPool
	if len(cfg.DeviceCert) > 0 {
		keyPair, err := tls.LoadX509KeyPair(cfg.DeviceCert, cfg.DeviceKey)
		if err != nil {
			logger.Errorf("error reading client certificate files - \"%s, %s\"", cfg.DeviceCert, cfg.DeviceKey)
			return nil, err
		}
		certificates = []tls.Certificate{keyPair}
	}
	if len(cfg.CaCert) > 0 {
		caCert, err := ioutil.ReadFile(cfg.CaCert)
		if err != nil {
			logger.Errorf("error reading CA certificate file - \"%s\"", cfg.CaCert)
			return nil, err
		}
		caCertPool = x509.NewCertPool()
		caCertPool.AppendCertsFromPEM(caCert)
	}
	config := &tls.Config{
		InsecureSkipVerify: false,
		RootCAs:            caCertPool,
		Certificates:       certificates,
		MinVersion:         tls.VersionTLS12,
		MaxVersion:         tls.VersionTLS13,
		CipherSuites:       uploaders.SupportedCipherSuites(),
	}
	opts := MQTT.NewClientOptions().
		AddBroker(cfg.Broker).
		SetClientID(uuid.New().String()).
		SetKeepAlive(30 * time.Second).
		SetCleanSession(true).
		SetAutoReconnect(true).SetTLSConfig(config)
	if len(cfg.Username) > 0 {
		opts = opts.SetUsername(cfg.Username).SetPassword(cfg.Password)
	}

	p := &EdgeConnector{mqttClient: MQTT.NewClient(opts), edgeClient: ecl}
	if token := p.mqttClient.Connect(); token.Wait() && token.Error() != nil {
		return nil, token.Error()
	}

	if token := p.mqttClient.Subscribe(topic, 1, func(client MQTT.Client, message MQTT.Message) {
		localCfg := &EdgeConfiguration{}
		err := json.Unmarshal(message.Payload(), localCfg)
		if err != nil {
			logger.Errorf("could not unmarshal edge configuration: %v", err)
			return
		}

		if p.cfg == nil || *localCfg != *p.cfg {
			logger.Infof("new edge configuration received: %v", localCfg)
			if p.cfg != nil {
				p.edgeClient.Disconnect()
			}
			p.cfg = localCfg
			ecl.Connect(p.mqttClient, p.cfg)
		}

	}); token.Wait() && token.Error() != nil {
		return nil, token.Error()
	}

	if token := p.mqttClient.Publish("edge/thing/request", 1, false, ""); token.Wait() && token.Error() != nil {
		return nil, token.Error()
	}

	return p, nil
}

// Close the EdgeConnector
func (p *EdgeConnector) Close() {
	if p.cfg != nil {
		p.edgeClient.Disconnect()
	}

	p.mqttClient.Unsubscribe(topic)
	p.mqttClient.Disconnect(200)

	logger.Info("disconnected from MQTT broker")
}
