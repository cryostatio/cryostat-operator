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
	"time"

	"github.com/gorilla/websocket"
	rhjmcv1alpha2 "github.com/rh-jmc-team/container-jfr-operator/pkg/apis/rhjmc/v1alpha2"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var log = logf.Log.WithName("containerjfr_client")
var debugLog = log.V(1)

const ioTimeout = 30 * time.Second

// Config stores configuration options to connect to Container JFR's
// command server
type Config struct {
	// URL to Container JFR's command server
	ServerURL   *url.URL
	AccessToken *string
	TLSVerify   bool
	UIDProvider func() types.UID
}

// ContainerJfrClient communicates with Container JFR's command server
// using a WebSocket connection
type ContainerJfrClient interface {
	ListRecordings(target *TargetAddress) ([]RecordingDescriptor, error)
	DumpRecording(target *TargetAddress, name string, seconds int, events []string) error
	StartRecording(target *TargetAddress, name string, events []string) error
	StopRecording(target *TargetAddress, name string) error
	DeleteRecording(target *TargetAddress, name string) error
	SaveRecording(target *TargetAddress, name string) (*string, error)
	ListSavedRecordings() ([]SavedRecording, error)
	DeleteSavedRecording(jfrFile string) error
	ListEventTypes(target *TargetAddress) ([]rhjmcv1alpha2.EventInfo, error)
	IsReady() (bool, error)
	Close() error
}

type containerJfrClient struct {
	config *Config
	conn   *websocket.Conn
}

// TargetAddress contains an address that Container JFR can use to connect
// to a particular JVM
type TargetAddress struct {
	Host string
	Port int32
}

// Create creates a ContainerJfrClient using the provided configuration
func Create(config *Config) (ContainerJfrClient, error) {
	configCopy := *config
	if config.ServerURL == nil {
		return nil, errors.New("ServerURL in config must not be nil")
	}
	if config.AccessToken == nil {
		return nil, errors.New("AccessToken in config must not be nil")
	}
	if config.UIDProvider == nil {
		configCopy.UIDProvider = uuid.NewUUID
	}
	conn, err := newWebSocketConn(config.ServerURL, config.AccessToken, config.TLSVerify)
	if err != nil {
		return nil, err
	}
	client := &containerJfrClient{config: &configCopy, conn: conn}
	return client, nil
}

// Close releases the WebSocket connection used by this client
func (client *containerJfrClient) Close() error {
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

func (target TargetAddress) String() string {
	return fmt.Sprintf("%s:%d", target.Host, target.Port)
}

// ListRecordings returns a list of its in-memory Flight Recordings
func (client *containerJfrClient) ListRecordings(target *TargetAddress) ([]RecordingDescriptor, error) {
	listCmd := NewCommandMessage("list", target.String(), client.newUID())
	recordings := []RecordingDescriptor{}
	err := client.syncMessage(listCmd, &recordings)
	if err != nil {
		return nil, err
	}
	log.Info("got list response", "resp", recordings)
	return recordings, nil
}

// DumpRecording instructs Container JFR to create a new recording of fixed duration
func (client *containerJfrClient) DumpRecording(target *TargetAddress, name string, seconds int,
	events []string) error {
	dumpCmd := NewCommandMessage("dump", target.String(), client.newUID(), name, strconv.Itoa(seconds),
		strings.Join(events, ","))
	var resp string
	err := client.syncMessage(dumpCmd, &resp)
	if err != nil {
		return err
	}
	log.Info("got dump response", "resp", resp)
	return nil
}

// StartRecording instructs Container JFR to create a new continuous recording
func (client *containerJfrClient) StartRecording(target *TargetAddress, name string, events []string) error {
	startCmd := NewCommandMessage("start", target.String(), client.newUID(), name, strings.Join(events, ","))
	var resp string
	err := client.syncMessage(startCmd, &resp)
	if err != nil {
		return err
	}
	log.Info("got start response", "resp", resp)
	return nil
}

// StopRecording instructs Container JFR to stop a recording
func (client *containerJfrClient) StopRecording(target *TargetAddress, name string) error {
	stopCmd := NewCommandMessage("stop", target.String(), client.newUID(), name)
	var resp string
	err := client.syncMessage(stopCmd, &resp)
	if err != nil {
		return err
	}
	log.Info("got stop response", "resp", resp)
	return nil
}

// DeleteRecording deletes a recording from Container JFR
func (client *containerJfrClient) DeleteRecording(target *TargetAddress, name string) error {
	deleteCmd := NewCommandMessage("delete", target.String(), client.newUID(), name)
	var resp string
	err := client.syncMessage(deleteCmd, &resp)
	if err != nil {
		return err
	}
	log.Info("got delete response", "resp", resp)
	return nil
}

// SaveRecording copies a flight recording file from local memory to persistent storage
func (client *containerJfrClient) SaveRecording(target *TargetAddress, name string) (*string, error) {
	saveCmd := NewCommandMessage("save", target.String(), client.newUID(), name)
	var resp string
	err := client.syncMessage(saveCmd, &resp)
	if err != nil {
		return nil, err
	}
	log.Info("got save response", "resp", resp)
	return &resp, nil
}

// ListSavedRecordings returns a list of recordings contained in persistent storage
func (client *containerJfrClient) ListSavedRecordings() ([]SavedRecording, error) {
	listCmd := NewControlMessage("list-saved", client.newUID())
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
func (client *containerJfrClient) DeleteSavedRecording(jfrFile string) error {
	deleteCmd := NewControlMessage("delete-saved", client.newUID(), jfrFile)
	var resp string
	err := client.syncMessage(deleteCmd, &resp)
	if err != nil {
		return err
	}
	log.Info("got delete response", "resp", resp)
	return nil
}

// ListEventTypes returns a list of events available in the target JVM
func (client *containerJfrClient) ListEventTypes(target *TargetAddress) ([]rhjmcv1alpha2.EventInfo, error) {
	listCmd := NewCommandMessage("list-event-types", target.String(), client.newUID())
	events := []rhjmcv1alpha2.EventInfo{}
	err := client.syncMessage(listCmd, &events)
	if err != nil {
		return nil, err
	}
	log.Info("got list-event-types response", "resp", events)
	return events, nil
}

func (client *containerJfrClient) IsReady() (bool, error) {
	return true, nil // XXX
}

func (client *containerJfrClient) syncMessage(msg *CommandMessage, responsePayload interface{}) error {
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

func (client *containerJfrClient) readResponse(msg *CommandMessage,
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

func (client *containerJfrClient) newUID() types.UID {
	return client.config.UIDProvider()
}
