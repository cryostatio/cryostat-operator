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
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-logr/logr"
	rhjmcv1alpha2 "github.com/rh-jmc-team/container-jfr-operator/pkg/apis/rhjmc/v1alpha2"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var log = logf.Log.WithName("containerjfr_client")

const ioTimeout = 30 * time.Second

// Config stores configuration options to connect to Container JFR's
// web server
type Config struct {
	// URL to Container JFR's web server
	ServerURL *url.URL
	// Bearer token to authenticate with Container JFR
	AccessToken *string
	// Certificate of CA to trust, in PEM format
	CACertificate []byte
}

// ContainerJfrClient contains methods for interacting with Container JFR's
// REST API
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
}

type httpClient struct {
	config *Config
	client *http.Client
}

type apiPath struct {
	resource string
	target   *TargetAddress
	name     *string
}

const (
	resRecordings     = "recordings"
	resEvents         = "events"
	attrRecordingName = "recordingName"
	attrEvents        = "events"
	attrDuration      = "duration"
	cmdStop           = "stop"
	cmdSave           = "save"
)

// NewHTTPClient creates a client to communicate with Container JFR over HTTP(S)
func NewHTTPClient(config *Config) (ContainerJfrClient, error) {
	configCopy := *config
	if config.ServerURL == nil {
		return nil, errors.New("ServerURL in config must not be nil")
	}
	if config.AccessToken == nil {
		return nil, errors.New("AccessToken in config must not be nil")
	}

	// Create CertPool for CA certificate
	var rootCAPool *x509.CertPool
	if config.CACertificate != nil {
		rootCAPool = x509.NewCertPool()
		ok := rootCAPool.AppendCertsFromPEM(config.CACertificate)
		if !ok {
			return nil, errors.New("Failed to parse CA certificate")
		}
	}

	// Use settings from default Transport with modified TLS config
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{
		RootCAs: rootCAPool,
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   ioTimeout,
	}
	log.Info("creating new Container JFR client", "server", config.ServerURL)
	return &httpClient{
		config: &configCopy,
		client: client,
	}, nil
}

// ListRecordings returns a list of its in-memory Flight Recordings
func (c *httpClient) ListRecordings(target *TargetAddress) ([]RecordingDescriptor, error) {
	path := &apiPath{
		resource: resRecordings,
		target:   target,
	}
	result := []RecordingDescriptor{}
	err := c.httpGet(path, &result)
	return result, err
}

// DumpRecording instructs Container JFR to create a new recording of fixed duration
func (c *httpClient) DumpRecording(target *TargetAddress, name string, seconds int, events []string) error {
	return c.postRecording(target, name, seconds, events)
}

// StartRecording instructs Container JFR to create a new continuous recording
func (c *httpClient) StartRecording(target *TargetAddress, name string, events []string) error {
	return c.postRecording(target, name, 0, events)
}

func (c *httpClient) postRecording(target *TargetAddress, name string, seconds int, events []string) error {
	path := &apiPath{
		resource: resRecordings,
		target:   target,
	}
	values := url.Values{}
	values.Add(attrRecordingName, name)
	values.Add(attrEvents, strings.Join(events, ","))
	if seconds > 0 {
		values.Add(attrDuration, strconv.Itoa(seconds))
	}
	result := RecordingDescriptor{} // TODO use this in reconciler to avoid get call
	err := c.httpPostForm(path, values, &result)
	return err
}

// StopRecording instructs Container JFR to stop a recording
func (c *httpClient) StopRecording(target *TargetAddress, name string) error {
	path := &apiPath{
		resource: resRecordings,
		target:   target,
		name:     &name,
	}
	return c.httpPatch(path, cmdStop, nil)
}

// DeleteRecording deletes a recording from Container JFR
func (c *httpClient) DeleteRecording(target *TargetAddress, name string) error {
	path := &apiPath{
		resource: resRecordings,
		target:   target,
		name:     &name,
	}
	return c.httpDelete(path, nil)
}

// SaveRecording copies a flight recording file from local memory to persistent storage
func (c *httpClient) SaveRecording(target *TargetAddress, name string) (*string, error) {
	path := &apiPath{
		resource: resRecordings,
		target:   target,
		name:     &name,
	}
	var result string
	err := c.httpPatch(path, cmdSave, &result)
	return &result, err
}

// ListSavedRecordings returns a list of recordings contained in persistent storage
func (c *httpClient) ListSavedRecordings() ([]SavedRecording, error) {
	path := &apiPath{
		resource: resRecordings,
	}
	result := []SavedRecording{}
	err := c.httpGet(path, &result)
	return result, err
}

// DeleteSavedRecording deletes a recording from the persistent storage managed
// by Container JFR
func (c *httpClient) DeleteSavedRecording(jfrFile string) error {
	path := &apiPath{
		resource: resRecordings,
		name:     &jfrFile,
	}
	return c.httpDelete(path, nil)
}

// ListEventTypes returns a list of events available in the target JVM
func (c *httpClient) ListEventTypes(target *TargetAddress) ([]rhjmcv1alpha2.EventInfo, error) {
	path := &apiPath{
		resource: resEvents,
		target:   target,
	}
	result := []rhjmcv1alpha2.EventInfo{}
	err := c.httpGet(path, &result)
	return result, err
}

func (c *httpClient) httpGet(path *apiPath, result interface{}) error {
	return c.sendRequest(http.MethodGet, path, nil, nil, result)
}

func (c *httpClient) httpPatch(path *apiPath, body string, result interface{}) error {
	contentType := "text/plain"
	return c.sendRequest(http.MethodPatch, path, strings.NewReader(body),
		&contentType, result)
}

func (c *httpClient) httpPostForm(path *apiPath, formData url.Values, result interface{}) error {
	contentType := "application/x-www-form-urlencoded"
	return c.sendRequest(http.MethodPost, path, strings.NewReader(formData.Encode()),
		&contentType, result)
}

func (c *httpClient) httpDelete(path *apiPath, result interface{}) error {
	return c.sendRequest(http.MethodDelete, path, nil, nil, result)
}

func (c *httpClient) sendRequest(method string, path *apiPath, body io.Reader, contentType *string,
	result interface{}) error {
	// Resolve API path with server URL
	pathURL, err := path.URL()
	if err != nil {
		return err
	}
	requestURL := c.config.ServerURL.ResolveReference(pathURL)
	httpLogger := log.WithValues("method", method, "url", requestURL)

	// Create request and set authorization header(s)
	req, err := http.NewRequest(method, requestURL.String(), body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+*c.config.AccessToken)
	if contentType != nil {
		req.Header.Set("Content-Type", *contentType)
	}

	// Send request to server
	httpLogger.Info("sending request")
	resp, err := c.client.Do(req)
	if err != nil {
		httpLogger.Error(err, "request error")
		return err
	}
	defer resp.Body.Close()

	// Convert non-2xx responses to errors
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Error response body will be plain text
		errMsg, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			httpLogger.Error(err, "failed to read error message from response body")
			return err
		}
		err = fmt.Errorf("server returned status: %s", resp.Status)
		httpLogger.Error(err, "request failed", "message", string(errMsg))
		return err
	}
	httpLogger.Info("request succeeded")

	// Decode response body stream directly
	return decodeResponse(resp.Body, result, httpLogger)
}

func decodeResponse(body io.Reader, result interface{}, httpLogger logr.Logger) error {
	httpDebug := httpLogger.V(1)
	if result != nil {
		// If result is of type string, expect response to be plain text
		resultStr, ok := result.(*string)
		if ok {
			buf, err := ioutil.ReadAll(body)
			if err != nil {
				httpLogger.Error(err, "could not parse plain text response")
				return err
			}
			*resultStr = string(buf)
			httpDebug.Info("parsed plain text response", "result", *resultStr)
		} else { // Otherwise, decode as JSON into struct
			err := json.NewDecoder(body).Decode(result)
			if err != nil {
				httpLogger.Error(err, "could not parse JSON response")
				return err
			}
			httpDebug.Info("parsed JSON response", "result", result)
		}
	}
	return nil
}

func (p *apiPath) URL() (*url.URL, error) {
	// Build path based on what fields are defined in the receiver
	var strPath string
	if p.target != nil {
		if p.name != nil {
			strPath = fmt.Sprintf("/api/v1/targets/%s/%s/%s", url.PathEscape(p.target.String()), p.resource, *p.name)
		} else {
			strPath = fmt.Sprintf("/api/v1/targets/%s/%s", url.PathEscape(p.target.String()), p.resource)
		}
	} else if p.name != nil {
		strPath = fmt.Sprintf("/api/v1/%s/%s", p.resource, *p.name)
	} else {
		strPath = fmt.Sprintf("/api/v1/%s", p.resource)
	}
	return url.Parse(strPath)
}
