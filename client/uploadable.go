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
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/eclipse-kanto/file-upload/logger"
	"github.com/eclipse-kanto/file-upload/uploaders"
	"github.com/eclipse/ditto-clients-golang"
	"github.com/eclipse/ditto-clients-golang/model"
	"github.com/eclipse/ditto-clients-golang/protocol"
	"github.com/eclipse/ditto-clients-golang/protocol/things"
	MQTT "github.com/eclipse/paho.mqtt.golang"
)

const (
	autoUploadProperty = "autoUpload"
	lastUploadProperty = "lastUpload"

	optionsPrefix = "options."

	filePathOption = "file.path"

	defaultDisconnectTimeout = 250 * time.Millisecond
	defaultKeepAlive         = 20 * time.Second
)

// UploadableConfig contains configuration for the AutoUploadable feature
type UploadableConfig struct {
	Name    string   `json:"name,omitempty" def:"{name}" descr:"Name for the {feature} feature.\nShould conform to https://docs.bosch-iot-suite.com/things/basic-concepts/namespace-thing-feature/#characters-allowed-in-a-feature-id"`
	Context string   `json:"context,omitempty" def:"edge" descr:"ID of the {feature} feature."`
	Type    string   `json:"type,omitempty" def:"file" descr:"Type of the {feature} feature."`
	Period  Duration `json:"period,omitempty" def:"10h" descr:"{period}. Should be a sequence of decimal numbers, each with optional fraction and a unit suffix, such as '300ms', '1.5h', '10m30s', etc. Valid time units are 'ns', 'us' (or 'µs'), 'ms', 's', 'm', 'h'"`

	Active     bool  `json:"active,omitempty" def:"false" descr:"Activate periodic {actions}"`
	ActiveFrom Xtime `json:"activeFrom,omitempty" descr:"Time from which periodic {actions} should be active, in RFC 3339 format (2006-01-02T15:04:05Z07:00). If omitted (and 'active' flag is set) current time will be used as start of the periodic {actions}."`
	ActiveTill Xtime `json:"activeTill,omitempty" descr:"Time till which periodic {actions} should be active, in RFC 3339 format (2006-01-02T15:04:05Z07:00). If omitted (and 'active' flag is set) periodic {actions} will be active indefinitely."`

	Delete       bool `json:"delete,omitempty" def:"false" descr:"Delete successfully uploaded files"`
	Checksum     bool `json:"checksum,omitempty" def:"false" descr:"Send MD5 checksum for uploaded files to ensure data integrity. Computing checksums incurs additional CPU/disk usage."`
	SingleUpload bool `json:"singleUpload,omitempty" def:"false" descr:"Forbid triggering of new uploads when there is upload in progress. Trigger can be forced from the backend with the 'force' option."`

	StopTimeout Duration `json:"stopTimeout,omitempty" def:"30s" descr:"Time to wait for running {running_actions} to finish when stopping. Should be a sequence of decimal numbers, each with optional fraction and a unit suffix, such as '300ms', '1.5h', '10m30s', etc. Valid time units are 'ns', 'us' (or 'µs'), 'ms', 's', 'm', 'h'"`
}

// AutoUploadableState is used for serializing the state property of the AutoUploadable feature
type AutoUploadableState struct {
	Active bool `json:"active"`

	StartTime *time.Time `json:"startTime"`
	EndTime   *time.Time `json:"endTime"`
}

// AutoUploadable feature implementation. Implements all required communication with the backend.
// Customized with UploadCustomizer
type AutoUploadable struct {
	client *ditto.Client

	deviceID string
	tenantID string

	info map[string]string

	state AutoUploadableState

	definitions []string
	cfg         *UploadableConfig

	customizer UploadCustomizer

	uidCounter int64

	statusEvents *statusEventsConsumer

	uploads *Uploads

	executor *PeriodicExecutor
	mutex    sync.Mutex
}

//ErrorResponse is returned from operations handling functions
type ErrorResponse struct {
	Status  int
	Message string
}

// UploadCustomizer is used to customize AutoUploadable behavior.
type UploadCustomizer interface {
	// DoTrigger is responsible for starting file uploads (by calling UploadFiles).
	// Called when trigger operation is invoked from the backend.
	DoTrigger(correlationID string, options map[string]string) error

	// HandleOperation is called when unknown operation is invoked from the backend.
	// Used when extending AutoUploadable with new operations
	HandleOperation(operation string, payload []byte) *ErrorResponse

	// OnTick is called by the periodic executor. Handles AutoUploadable period tasks.
	OnTick()
}

// Validate checks configuration validity
func (cfg *UploadableConfig) Validate() {
	if cfg.Period <= 0 {
		log.Fatalln("Period should be larger than zero!")
	}

	if cfg.ActiveFrom.Time != nil || cfg.ActiveTill.Time != nil {
		if cfg.ActiveFrom.Time != nil && cfg.ActiveTill.Time != nil && cfg.ActiveTill.Time.Before(*cfg.ActiveFrom.Time) {
			log.Fatalf("'activeFrom' time should be before 'activeTill' time")
		}

		cfg.Active = true
	}
}

// NewAutoUploadable constructs AutoUploadable from the provided configurations
func NewAutoUploadable(mqttClient MQTT.Client, edgeCfg *EdgeConfiguration, uploadableCfg *UploadableConfig, handler UploadCustomizer, definitions ...string) (*AutoUploadable, error) {
	result := &AutoUploadable{}

	result.customizer = handler
	result.definitions = definitions

	config := ditto.NewConfiguration().
		WithDisconnectTimeout(defaultDisconnectTimeout).
		WithConnectHandler(
			func(client *ditto.Client) {
				result.connectHandler(client)
			})

	var err error
	result.client, err = ditto.NewClientMqtt(mqttClient, config)
	if err != nil {
		return nil, err
	}

	result.deviceID = edgeCfg.DeviceID
	result.tenantID = edgeCfg.TenantID

	result.cfg = uploadableCfg
	result.uidCounter = time.Now().Unix()

	result.statusEvents = newStatusEventsConsumer(100)

	result.state.Active = uploadableCfg.Active
	result.state.StartTime = uploadableCfg.ActiveFrom.Time
	result.state.EndTime = uploadableCfg.ActiveTill.Time

	result.info = map[string]string{"supportedProviders": uploaders.StorageProviderAWS + "," + uploaders.StorageProviderAzure + "," + uploaders.StorageProviderHTTP}

	result.uploads = NewUploads()

	result.client.Subscribe(func(requestID string, msg *protocol.Envelope) {
		go result.messageHandler(requestID, msg)
	})

	return result, nil
}

// Connect AutoUploadable to the Ditto endpoint
func (u *AutoUploadable) Connect() error {
	u.statusEvents.start(func(e interface{}) {
		u.UpdateProperty(lastUploadProperty, e)
	})

	return u.client.Connect()
}

// Disconnect AutoUploadable from the Ditto endpoint and clean up used resources
func (u *AutoUploadable) Disconnect() {
	u.statusEvents.stop()

	u.client.Unsubscribe()

	u.stopExecutor() //stop periodic triggers

	u.uploads.Stop(time.Duration(u.cfg.StopTimeout)) // stop active uploads

	u.client.Disconnect()
}

func (u *AutoUploadable) connectHandler(client *ditto.Client) {
	feature := &model.Feature{}

	feature.WithDefinitionFrom(u.definitions...).
		WithProperty("type", u.cfg.Type).WithProperty("context", u.cfg.Context).WithProperty("info", u.info).WithProperty(autoUploadProperty, u.state)

	cmd := things.NewCommand(model.NewNamespacedIDFrom(u.deviceID)).Twin().Feature(u.cfg.Name).Modify(feature)
	msg := cmd.Envelope(protocol.WithResponseRequired(false))

	err := client.Send(msg)
	if err != nil {
		panic(fmt.Errorf("failed to create '%s' feature", u.cfg.Name))
	}

	if u.cfg.Active {
		u.startExecutor()
	}
}

func (u *AutoUploadable) sendUploadRequest(correlationID string, options map[string]string, filePath string) {
	type uploadRequest struct {
		CorrelationID string            `json:"correlationId"`
		Options       map[string]string `json:"options"`
	}

	request := uploadRequest{correlationID, options}

	msg := things.NewMessage(model.NewNamespacedIDFrom(u.deviceID)).Feature(u.cfg.Name).Outbox("request").WithPayload(request)

	replyTo := fmt.Sprintf("command/%s", u.tenantID)
	err := u.client.Send(msg.Envelope(protocol.WithResponseRequired(false), protocol.WithContentType("application/json"), protocol.WithReplyTo(replyTo)))

	if err != nil {
		logger.Errorf("failed to send request upload message '%v' for file '%s': %v", request, filePath, err)
	} else {
		logger.Infof("request upload message '%v' sent for file '%s'", msg, filePath)
	}
}

func (e *ErrorResponse) Error() string {
	return fmt.Sprintf("error response [code=%d, msg=%s]", e.Status, e.Message)
}

// messageHandler should be called in separate go routine for each request
func (u *AutoUploadable) messageHandler(requestID string, msg *protocol.Envelope) {
	if !strings.HasPrefix(msg.Path, "/features/"+u.cfg.Name) {
		return //not for me
	}

	logger.Infof("message received: path:=%s, topic=%s, value=%v", msg.Path, msg.Topic, msg.Value)

	if model.NewNamespacedID(msg.Topic.Namespace, msg.Topic.EntityID).String() != u.deviceID {
		return
	}

	value, ok := (msg.Value).(map[string]interface{})
	if !ok && msg.Value != nil {
		logger.Errorf("unexpected message type: %T", msg.Value)
		return
	}

	payload, err := json.Marshal(value)
	if err != nil {
		logger.Errorf("could not parse message value: %v", msg.Value)
	}

	operationPrefix := "/features/" + u.cfg.Name + "/inbox/messages/"
	operation := strings.TrimPrefix(msg.Path, operationPrefix)

	if operation == msg.Path { //wrong prefix
		logger.Warningf("ignoring unsupported message '%v'", msg.Topic)
		return
	}

	responseError := (*ErrorResponse)(nil)

	switch operation {
	case "start":
		responseError = u.start(payload)
	case "trigger":
		responseError = u.trigger(payload)
	case "cancel":
		responseError = u.cancel(payload)
	case "activate":
		responseError = u.activate(payload)
	case "deactivate":
		responseError = u.deactivate(payload)
	default:
		responseError = u.customizer.HandleOperation(operation, payload)
	}

	status := http.StatusNoContent
	message := interface{}(nil)

	if responseError != nil {
		status = responseError.Status
		message = responseError.Message

		logger.Errorf("error while executing operation %s: %s", operation, responseError.Message)
	}

	reply := &protocol.Envelope{
		Topic:   msg.Topic,                                           // preserve the topic
		Headers: msg.Headers,                                         // preserve the headers
		Path:    strings.Replace(msg.Path, "/inbox/", "/outbox/", 1), // switch to outbox
		Value:   message,                                             // fill the response value
		Status:  status,                                              // set the response status
	}

	u.client.Reply(requestID, reply)
}

// UpdateProperty sends Ditto message for value update of the given property
func (u *AutoUploadable) UpdateProperty(name string, value interface{}) {
	command := things.NewCommand(model.NewNamespacedIDFrom(u.deviceID)).Twin().FeatureProperty(u.cfg.Name, name).Modify(value)

	envelope := command.Envelope(protocol.WithResponseRequired(false))

	if err := u.client.Send(envelope); err != nil {
		logger.Errorf("could not send Ditto message: %v", err)
	} else {
		logger.Infof("feature property '%s' value updated: %v", name, value)
	}
}

// ******* AutoUploadable Feature operations *******//

// ******* UploadStatusListener methods *******//

func (u *AutoUploadable) uploadStatusUpdated(status *UploadStatus) {
	defer func() {
		if e := recover(); e != nil {
			logger.Warning(e) //already closed
		}
	}()

	s := *status
	u.statusEvents.add(s)
}

// ******* END UploadStatusListener methods *******//

func (u *AutoUploadable) activate(payload []byte) *ErrorResponse {
	type inputParams struct {
		From *time.Time `json:"from"`
		To   *time.Time `json:"to"`
	}
	params := &inputParams{}
	err := json.Unmarshal(payload, params)
	if err != nil {
		msg := fmt.Sprintf("invalid 'activate' operation parameters: %v", string(payload))
		return &ErrorResponse{http.StatusBadRequest, msg}
	}

	if params.To.Before(*params.From) {
		msg := fmt.Sprintf("period end - %v -  is before period start - %v", params.To, params.From)
		return &ErrorResponse{http.StatusBadRequest, msg}
	}

	logger.Infof("activate called: %+v", params)
	u.state.Active = true
	u.state.StartTime = params.From
	u.state.EndTime = params.To

	u.startExecutor()

	u.UpdateProperty(autoUploadProperty, u.state)

	return nil
}

func (u *AutoUploadable) deactivate(payload []byte) *ErrorResponse {
	logger.Info("deactivate called")
	u.state.Active = false
	u.state.StartTime = nil
	u.state.EndTime = nil

	u.stopExecutor()

	u.UpdateProperty(autoUploadProperty, u.state)

	return nil
}

func (u *AutoUploadable) trigger(payload []byte) *ErrorResponse {
	type inputParams struct {
		CorrelationID string            `json:"correlationId"`
		Options       map[string]string `json:"options"`
	}
	params := &inputParams{}

	err := json.Unmarshal(payload, params)
	if err != nil {
		msg := fmt.Sprintf("invalid 'trigger' operation parameters: %v", string(payload))
		return &ErrorResponse{http.StatusBadRequest, msg}
	}

	logger.Infof("trigger called: %+v", params)

	correlationID := params.CorrelationID
	if correlationID == "" {
		correlationID = u.nextgUID()
	}

	err = u.customizer.DoTrigger(correlationID, params.Options)
	if err != nil {
		return &ErrorResponse{http.StatusInternalServerError, err.Error()}
	}

	return nil
}

func (u *AutoUploadable) start(payload []byte) *ErrorResponse {
	type inputParams struct {
		CorrelationID string            `json:"correlationId"`
		Options       map[string]string `json:"options"`
	}
	params := &inputParams{}

	err := json.Unmarshal(payload, params)
	if err != nil {
		msg := fmt.Sprintf("invalid 'start' operation parameters: %v", string(payload))
		return &ErrorResponse{http.StatusBadRequest, msg}
	}

	logger.Infof("start called: %+v", params)

	up := u.uploads.Get(params.CorrelationID)

	if up == nil {
		return &ErrorResponse{http.StatusNotFound, fmt.Sprintf("upload with correlation ID '%s' not found", params.CorrelationID)}
	}

	err = up.start(params.Options)
	if err != nil {
		logger.Errorf("failed to start upload %s: %v", params.CorrelationID, err)
		return &ErrorResponse{http.StatusInternalServerError, err.Error()}
	}

	return nil
}

func (u *AutoUploadable) cancel(payload []byte) *ErrorResponse {
	type inputParams struct {
		CorrelationID string `json:"correlationId"`
		StatusCode    string `json:"statusCode"`
		Message       string `json:"message"`
	}
	params := &inputParams{}

	err := json.Unmarshal(payload, params)
	if err != nil {
		msg := fmt.Sprintf("invalid 'cancel' operation parameters: %v", string(payload))
		return &ErrorResponse{http.StatusBadRequest, msg}
	}

	logger.Infof("cancel called: %+v", params)

	up := u.uploads.Get(params.CorrelationID)
	if up == nil {
		logger.Errorf("failed to cancel upload %s: %v", params.CorrelationID, err)
		return &ErrorResponse{http.StatusNotFound, fmt.Sprintf("upload with correlation ID '%s' not found", params.CorrelationID)}
	}

	go up.cancel(params.StatusCode, params.Message)

	return nil
}

// ******* END AutoUploadable Feature operations *******//

// UploadFiles starts the upload of the given files, by sending an upload request with the specified
// correlation ID and options.
func (u *AutoUploadable) UploadFiles(correlationID string, files []string, options map[string]string) {
	childIDs := u.uploads.AddMulti(correlationID, files, u.cfg.Delete, u.cfg.Checksum, u)
	for i, childID := range childIDs {
		options := uploaders.ExtractDictionary(options, optionsPrefix)
		options["storage.providers"] = "aws, azure, generic"
		options[filePathOption] = files[i]

		go u.sendUploadRequest(childID, options, files[i])
	}
}

func (u *AutoUploadable) startExecutor() {
	u.mutex.Lock()
	defer u.mutex.Unlock()

	if u.executor != nil {
		u.executor.Stop()
	}

	u.executor = NewPeriodicExecutor(u.state.StartTime, u.state.EndTime, time.Duration(u.cfg.Period), func() {
		u.customizer.OnTick()
	})
}

func (u *AutoUploadable) stopExecutor() {
	u.mutex.Lock()
	defer u.mutex.Unlock()

	if u.executor != nil {
		u.executor.Stop()
		u.executor = nil
	}
}

func (u *AutoUploadable) nextgUID() string {
	u.mutex.Lock()
	defer u.mutex.Unlock()

	u.uidCounter++

	return fmt.Sprintf("upload-id-%d", u.uidCounter)
}
