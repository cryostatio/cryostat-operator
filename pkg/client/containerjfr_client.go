// Copyright (c) 2020 Red Hat, Inc.
//
// The Universal Permissive License (UPL), Version 1.0
//
// Subject to the condition set forth below, permission is hereby granted to any
// person obtaining a copy of this software, associated documentation and/or data
// (collectively the "Software"), free of charge and under any and all copyright
// rights in the Software, and any and all patent rights owned or freely
// licensable by each licensor hereunder covering either (i) the unmodified
// Software as contributed to or provided by such licensor, or (ii) the Larger
// Works (as defined below), to deal in both
//
// (a) the Software, and
// (b) any piece of software and/or hardware listed in the lrgrwrks.txt file if
// one is included with the Software (each a "Larger Work" to which the Software
// is contributed by such licensors),
//
// without restriction, including without limitation the rights to copy, create
// derivative works of, display, perform, and distribute the Software and make,
// use, sell, offer for sale, import, export, have made, and have sold the
// Software and the Larger Work(s), and to sublicense the foregoing rights on
// either these or other terms.
//
// This license is subject to the following condition:
// The above copyright notice and either this complete permission notice or at
// a minimum a reference to the UPL must be included in all copies or
// substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package client

import (
	"crypto/tls"
	b64 "encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	rhjmcv1alpha2 "github.com/rh-jmc-team/container-jfr-operator/pkg/apis/rhjmc/v1alpha2"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var log = logf.Log.WithName("containerjfr_client")
var debugLog = log.V(1)

const ioTimeout = 30 * time.Second

// ClientLock synchronizes access to Container JFR while it is operating on a particular JVM
var ClientLock = &sync.Mutex{}
var targetHost *string = nil
var targetPort *int32 = new(int32)

// Config stores configuration options to connect to Container JFR's
// command server
type Config struct {
	// URL to Container JFR's command server
	ServerURL   *url.URL
	AccessToken *string
	TLSVerify   bool
}

// ContainerJfrClient communicates with Container JFR's command server
// using a WebSocket connection
type ContainerJfrClient struct {
	config *Config
	conn   *websocket.Conn
}

// Create creates a ContainerJfrClient using the provided configuration
func Create(config *Config) (*ContainerJfrClient, error) {
	if config.ServerURL == nil {
		return nil, errors.New("ServerURL in config must not be nil")
	}
	if config.AccessToken == nil {
		return nil, errors.New("AccessToken in config must not be nil")
	}
	conn, err := newWebSocketConn(config.ServerURL, config.AccessToken, config.TLSVerify)
	if err != nil {
		return nil, err
	}
	client := &ContainerJfrClient{config: config, conn: conn}
	return client, nil
}

// Close releases the WebSocket connection used by this client
func (client *ContainerJfrClient) Close() error {
	return client.conn.Close()
}

func newWebSocketConn(server *url.URL, token *string, tlsVerify bool) (*websocket.Conn, error) {
	b64tok := b64.StdEncoding.EncodeToString([]byte(*token))
	dialer := &websocket.Dialer{
		Proxy:            websocket.DefaultDialer.Proxy,
		HandshakeTimeout: websocket.DefaultDialer.HandshakeTimeout,
		TLSClientConfig:  &tls.Config{InsecureSkipVerify: !tlsVerify},
		Subprotocols:     []string{"base64url.bearer.authorization.containerjfr." + b64tok},
	}
	conn, _, err := dialer.Dial(server.String(), nil)
	if err != nil {
		log.Error(err, "failed to connect to command channel", "server", server.String())
		return nil, err
	}
	return conn, nil
}

// Connect tells Container JFR to connect to a JVM addressed by the host and port
func (client *ContainerJfrClient) Connect(host string, port int32) error {
	targetHost = &host
	targetPort = &port
	return nil
}

// Disconnect tells Container JFR to disconnect from its target JVM
func (client *ContainerJfrClient) Disconnect() error {
	targetHost = nil
	targetPort = new(int32)
	return nil
}

func TargetID() string {
	return fmt.Sprintf("%s:%d", *targetHost, *targetPort)
}

// ListRecordings returns a list of its in-memory Flight Recordings
func (client *ContainerJfrClient) ListRecordings() ([]RecordingDescriptor, error) {
	listCmd := NewCommandMessage("list", TargetID())
	recordings := []RecordingDescriptor{}
	err := client.syncMessage(listCmd, &recordings)
	if err != nil {
		return nil, err
	}
	log.Info("got list response", "resp", recordings)
	return recordings, nil
}

// DumpRecording instructs Container JFR to create a new recording of fixed duration
func (client *ContainerJfrClient) DumpRecording(name string, seconds int, events []string) error {
	dumpCmd := NewCommandMessage("dump", TargetID(), name, strconv.Itoa(seconds), strings.Join(events, ","))
	var resp string
	err := client.syncMessage(dumpCmd, &resp)
	if err != nil {
		return err
	}
	log.Info("got dump response", "resp", resp)
	return nil
}

// StartRecording instructs Container JFR to create a new continuous recording
func (client *ContainerJfrClient) StartRecording(name string, events []string) error {
	startCmd := NewCommandMessage("start", TargetID(), name, strings.Join(events, ","))
	var resp string
	err := client.syncMessage(startCmd, &resp)
	if err != nil {
		return err
	}
	log.Info("got start response", "resp", resp)
	return nil
}

// StopRecording instructs Container JFR to stop a recording
func (client *ContainerJfrClient) StopRecording(name string) error {
	stopCmd := NewCommandMessage("stop", TargetID(), name)
	var resp string
	err := client.syncMessage(stopCmd, &resp)
	if err != nil {
		return err
	}
	log.Info("got stop response", "resp", resp)
	return nil
}

// DeleteRecording deletes a recording from Container JFR
func (client *ContainerJfrClient) DeleteRecording(name string) error {
	deleteCmd := NewCommandMessage("delete", TargetID(), name)
	var resp string
	err := client.syncMessage(deleteCmd, &resp)
	if err != nil {
		return err
	}
	log.Info("got delete response", "resp", resp)
	return nil
}

// SaveRecording copies a flight recording file from local memory to persistent storage
func (client *ContainerJfrClient) SaveRecording(name string) (*string, error) {
	saveCmd := NewCommandMessage("save", TargetID(), name)
	var resp string
	err := client.syncMessage(saveCmd, &resp)
	if err != nil {
		return nil, err
	}
	log.Info("got save response", "resp", resp)
	return &resp, nil
}

// ListSavedRecordings returns a list of recordings contained in persistent storage
func (client *ContainerJfrClient) ListSavedRecordings() ([]SavedRecording, error) {
	listCmd := NewControlMessage("list-saved")
	recordings := []SavedRecording{}
	err := client.syncMessage(listCmd, &recordings)
	if err != nil {
		return nil, err
	}
	log.Info("got list-saved response", "resp", recordings)
	return recordings, nil
}

// DeleteSavedRecording deletes a recording from the persistent storage managed
// by Container JFR
func (client *ContainerJfrClient) DeleteSavedRecording(jfrFile string) error {
	deleteCmd := NewControlMessage("delete-saved", jfrFile)
	var resp string
	err := client.syncMessage(deleteCmd, &resp)
	if err != nil {
		return err
	}
	log.Info("got delete response", "resp", resp)
	return nil
}

// ListEventTypes returns a list of events available in the target JVM
func (client *ContainerJfrClient) ListEventTypes() ([]rhjmcv1alpha2.EventInfo, error) {
	listCmd := NewCommandMessage("list-event-types", TargetID())
	events := []rhjmcv1alpha2.EventInfo{}
	err := client.syncMessage(listCmd, &events)
	if err != nil {
		return nil, err
	}
	log.Info("got list-event-types response", "resp", events)
	return events, nil
}

func (client *ContainerJfrClient) syncMessage(msg *CommandMessage, responsePayload interface{}) error {
	client.conn.SetWriteDeadline(time.Now().Add(ioTimeout))
	err := client.conn.WriteJSON(msg)
	if err != nil {
		log.Error(err, "could not write message", "message", msg)
		return err
	}
	log.Info("sent command", "json", msg)

	resp, err := client.readResponse(msg, responsePayload)
	if err != nil {
		log.Error(err, "could not read response", "message", msg)
		return err
	}
	log.Info("got response", "resp", resp)
	if resp.Status < 0 {
		// Parse exception/failure response and convert to error
		errMsg, ok := resp.Payload.(string)
		if !ok {
			errMsg = "unknown error response"
		}
		err = fmt.Errorf("server failed to execute \"%s\": %s (code %d)", resp.CommandName, errMsg, resp.Status)
		log.Error(err, "command failed", "request", msg)
		return err
	}
	return err
}

func (client *ContainerJfrClient) readResponse(msg *CommandMessage,
	responsePayload interface{}) (*ResponseMessage, error) {
	// Set the original message ID so our decoder can verify it.
	// By setting the output argument in the struct, the decoder knows what type
	// to unmarshall the payload into and also stores it for returning to the caller.
	resp := &ResponseMessage{ID: msg.ID, Payload: responsePayload}

	// Time out if we don't get our response in a reasonable timeframe
	loopDeadline := time.Now().Add(ioTimeout)

	// Keep reading messages until we find a response to this message,
	// or encounter an error other than ErrWrongID.
	for time.Now().Before(loopDeadline) {
		client.conn.SetReadDeadline(time.Now().Add(ioTimeout))
		err := client.conn.ReadJSON(resp)
		if err == nil || err != ErrWrongID {
			return resp, err
		}
	}
	return nil, errors.New("Timed out waiting for a response")
}
