// Copyright (c) 2022 Contributors to the Eclipse Foundation
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
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eclipse/ditto-clients-golang/protocol"
	MQTT "github.com/eclipse/paho.mqtt.golang"
)

const (
	namespace = "testNamespace"
	deviceID  = "testDeviceID"
	featureID = "TestUpload"
)

var (
	basedir string
	testCfg *UploadableConfig
)

func setUp(t *testing.T) {
	var err error

	basedir, err = os.MkdirTemp(".", "testdir")
	if err != nil {
		t.Fatal(err)
	}
}

func tearDown(t *testing.T) {
	if err := os.RemoveAll(basedir); err != nil {
		t.Log(err)
	}
}

func getTestFiles(t *testing.T) (string, string, string, string) {
	return addTestFile(t, "a.txt"), addTestFile(t, "b.txt"), addTestFile(t, "c.dat"), addTestFile(t, "d.dat")
}

func TestUpload(t *testing.T) {
	setUp(t)
	defer tearDown(t)

	a, b, _, _ := getTestFiles(t)
	glob := filepath.Join(basedir, "*.txt")

	f, client := newConnectedFileUpload(t, glob)
	defer f.Disconnect()

	checkUploadTrigger(t, f, client, nil, a, b)
}

func TestUploadDynamicGlob(t *testing.T) {
	setUp(t)
	defer tearDown(t)

	a, b, c, d := getTestFiles(t)
	glob := filepath.Join(basedir, "*.txt")

	f, client := newConnectedFileUpload(t, glob)
	defer f.Disconnect()

	checkUploadTrigger(t, f, client, nil, a, b)

	dynamicGlob := filepath.Join(basedir, "*.dat")
	options := map[string]string{uploadFilesProperty: dynamicGlob}
	checkUploadTrigger(t, f, client, options, c, d)

	options[uploadFilesProperty] = "*.none"
	checkUploadTrigger(t, f, client, options)
}

func TestUploadDynamicGlobError(t *testing.T) {
	f, _ := newConnectedFileUpload(t, "")
	defer f.Disconnect()

	var err error

	err = f.DoTrigger("testCorrelationID", nil)
	assertError(t, err)

	options := map[string]string{uploadFilesProperty: "*.txt"}
	err = f.DoTrigger("testCorrelationID", options)
	assertNoError(t, err)
}

func checkUploadTrigger(t *testing.T, f *FileUpload, client *mockedClient, options map[string]string, expected ...string) {
	t.Helper()

	err := f.DoTrigger("testCorrelationID", options)
	assertNoError(t, err)

	var actual []string

	if len(expected) > 0 {
		actual = make([]string, len(expected))
	}

	for i := range actual {
		msg := client.liveMsg(t, request)
		file := getFileFromMsg(t, msg)

		actual[i] = file
	}

	client.assertLiveEmpty(t)

	sort.Strings(expected)
	sort.Strings(actual)

	assertEquals(t, expected, actual)
}

func addTestFile(t *testing.T, path string) string {
	t.Helper()

	dir := filepath.Dir(path)
	dir = filepath.Join(basedir, dir)

	err := os.MkdirAll(dir, 0700)
	assertNoError(t, err)

	path = filepath.Join(basedir, path)

	err = os.WriteFile(path, ([]byte)(path), 0666)
	assertNoError(t, err)

	return path
}

func newConnectedFileUpload(t *testing.T, filesGlob string) (*FileUpload, *mockedClient) {
	testCfg = &UploadableConfig{}
	testCfg.Name = featureID
	testCfg.Type = "test_type"
	testCfg.Context = "test_context"

	client := newMockedClient()
	edgeCfg := &EdgeConfiguration{DeviceID: namespace + ":" + deviceID, TenantID: "testTenantID", PolicyID: "testPolicyID"}

	var err error
	u, err := NewFileUpload(filesGlob, client, edgeCfg, testCfg)
	assertNoError(t, err)

	err = u.Connect()
	assertNoError(t, err)

	v := client.twinMsg(t, modify)
	props := v["properties"].(map[string]interface{})
	assertEquals(t, testCfg.Type, props["type"])
	assertEquals(t, testCfg.Context, props["context"])

	return u, client
}

func getFileFromMsg(t *testing.T, v map[string]interface{}) string {
	requestOptions, ok := v["options"].(map[string]interface{})
	if !ok {
		t.Fatalf("unexpected 'options' type: %T", v["options"])
	}

	file := requestOptions[filePathOption].(string)

	return file
}

func assertError(t *testing.T, err error) {
	t.Helper()

	if err == nil {
		t.Error("error expected, but there was none")
	}
}

const (
	twin = "twin"
	live = "live"

	modify  = "modify"
	request = "request"
)

// mockedClient represents mocked MQTT.Client interface used for testing.
type mockedClient struct {
	err  error
	twin chan *protocol.Envelope
	live chan *protocol.Envelope
	mu   sync.Mutex
}

func newMockedClient() *mockedClient {
	client := &mockedClient{}

	client.twin = make(chan *protocol.Envelope, 10)
	client.live = make(chan *protocol.Envelope, 10)

	return client
}

func (client *mockedClient) twinMsg(t *testing.T, action string) map[string]interface{} {
	t.Helper()

	return client.msg(t, twin, modify)
}

func (client *mockedClient) liveMsg(t *testing.T, action string) map[string]interface{} {
	t.Helper()

	return client.msg(t, live, action)
}

func (client *mockedClient) assertLiveEmpty(t *testing.T) {
	t.Helper()

	client.assertEmpty(t, live)
}

// msg returns last payload value or waits 5 seconds for new payload.
func (client *mockedClient) msg(t *testing.T, channel string, action string) map[string]interface{} {
	t.Helper()
	client.mu.Lock()
	defer client.mu.Unlock()

	ch := client.getChannel(channel)
	select {
	case env := <-ch:
		assertEquals(t, namespace, env.Topic.Namespace)
		assertEquals(t, deviceID, env.Topic.EntityID)
		assertEquals(t, action, string(env.Topic.Action))

		// Valdiate its starting path.
		prefix := "/features/" + featureID
		if !strings.HasPrefix(env.Path, prefix) {
			t.Fatalf("message path do not starts with [%v]: %v", prefix, env.Path)
		}
		// Return its the value.

		m, ok := env.Value.(map[string]interface{})
		if !ok {
			t.Fatalf("unexpected payload type: %T", m)
		}
		return m
	case <-time.After(5 * time.Second):
		// Fail after the timeout.
		t.Fatal("failed to retrieve published data")
	}
	return nil
}

func (client *mockedClient) assertEmpty(t *testing.T, channel string) {
	t.Helper()
	client.mu.Lock()
	defer client.mu.Unlock()

	ch := client.getChannel(channel)
	select {
	case env := <-ch:
		t.Fatalf("no more messages expected, but received %+v", env)
	case <-time.After(100 * time.Millisecond):
		return
	}
}

func (client *mockedClient) getChannel(channel string) chan *protocol.Envelope {
	if channel == twin {
		return client.twin
	} else if channel == live {
		return client.live
	}

	log.Fatalf("unknown channel: %s", channel)
	return nil
}

// IsConnected returns true.
func (client *mockedClient) IsConnected() bool {
	return true
}

// IsConnectionOpen returns true.
func (client *mockedClient) IsConnectionOpen() bool {
	return true
}

// Connect returns finished token.
func (client *mockedClient) Connect() MQTT.Token {
	return &mockedToken{err: client.err}
}

// Disconnect do nothing.
func (client *mockedClient) Disconnect(quiesce uint) {
	// Do nothing.
}

// Publish returns finished token and set client topic and payload.
func (client *mockedClient) Publish(topic string, qos byte, retained bool, payload interface{}) MQTT.Token {
	env := &protocol.Envelope{}
	if err := json.Unmarshal(payload.([]byte), env); err != nil {
		log.Fatalf("unexpected error during data unmarshal: %v", err)
	}

	if env.Topic.Channel == live {
		client.live <- env
	} else if env.Topic.Channel == twin {
		client.twin <- env
	} else {
		log.Fatalf("unexpected message topic: %v", env.Topic)
	}

	return &mockedToken{err: client.err}
}

// Subscribe returns finished token.
func (client *mockedClient) Subscribe(topic string, qos byte, callback MQTT.MessageHandler) MQTT.Token {
	return &mockedToken{err: client.err}
}

// SubscribeMultiple returns finished token.
func (client *mockedClient) SubscribeMultiple(filters map[string]byte, callback MQTT.MessageHandler) MQTT.Token {
	return &mockedToken{err: client.err}
}

// Unsubscribe returns finished token.
func (client *mockedClient) Unsubscribe(topics ...string) MQTT.Token {
	return &mockedToken{err: client.err}
}

// AddRoute do nothing.
func (client *mockedClient) AddRoute(topic string, callback MQTT.MessageHandler) {
	// Do nothing.
}

// OptionsReader returns an empty struct.
func (client *mockedClient) OptionsReader() MQTT.ClientOptionsReader {
	return MQTT.ClientOptionsReader{}
}

// mockedToken represents mocked MQTT.Token interface used for testing.
type mockedToken struct {
	err error
}

// Wait returns immediately with true.
func (token *mockedToken) Wait() bool {
	return true
}

// WaitTimeout returns immediately with true.
func (token *mockedToken) WaitTimeout(time.Duration) bool {
	return true
}

// Done returns immediately with nil channel.
func (token *mockedToken) Done() <-chan struct{} {
	return nil
}

// Error returns the error if set.
func (token *mockedToken) Error() error {
	return token.err
}
