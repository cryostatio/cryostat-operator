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
)

type httpClient struct {
	config *Config
	client *http.Client
}

const (
	attrRecordingName = "recordingName"
	attrEvents        = "events"
	attrDuration      = "duration"
)

func NewHTTPClient(config *Config) (ContainerJfrClient, error) {
	configCopy := *config
	if config.ServerURL == nil {
		return nil, errors.New("ServerURL in config must not be nil")
	}
	if config.AccessToken == nil {
		return nil, errors.New("AccessToken in config must not be nil")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: !config.TLSVerify}
	client := &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}
	return &httpClient{
		config: &configCopy,
		client: client,
	}, nil
}

func (c *httpClient) ListRecordings(target *TargetAddress) ([]RecordingDescriptor, error) {
	result := []RecordingDescriptor{}
	err := c.httpGet(apiPath("recordings", nil, target), &result)
	return result, err
}

func (c *httpClient) DumpRecording(target *TargetAddress, name string, seconds int, events []string) error {
	return c.postRecording(target, name, seconds, events)
}

func (c *httpClient) StartRecording(target *TargetAddress, name string, events []string) error {
	return c.postRecording(target, name, 0, events)
}

func (c *httpClient) postRecording(target *TargetAddress, name string, seconds int, events []string) error {
	values := url.Values{}
	values.Add(attrRecordingName, name)
	values.Add(attrEvents, strings.Join(events, ","))
	if seconds > 0 {
		values.Add(attrDuration, strconv.Itoa(seconds))
	}
	result := RecordingDescriptor{} // TODO use this to avoid get call
	err := c.httpPostForm(apiPath("recordings", nil, target), values, &result)
	return err
}

func (c *httpClient) StopRecording(target *TargetAddress, name string) error {
	return c.httpPatch(apiPath("recordings", &name, target), "stop", nil)
}

func (c *httpClient) DeleteRecording(target *TargetAddress, name string) error {
	return c.httpDelete(apiPath("recordings", &name, target), nil)
}

func (c *httpClient) SaveRecording(target *TargetAddress, name string) (*string, error) {
	var result string
	err := c.httpPatch(apiPath("recordings", &name, target), "save", &result)
	return &result, err
}

func (c *httpClient) ListSavedRecordings() ([]SavedRecording, error) {
	result := []SavedRecording{}
	err := c.httpGet(apiPath("recordings", nil, nil), &result)
	return result, err
}

func (c *httpClient) DeleteSavedRecording(jfrFile string) error {
	return c.httpDelete(apiPath("recordings", &jfrFile, nil), nil)
}

func (c *httpClient) ListEventTypes(target *TargetAddress) ([]rhjmcv1alpha2.EventInfo, error) {
	result := []rhjmcv1alpha2.EventInfo{}
	err := c.httpGet(apiPath("events", nil, target), &result)
	return result, err
}

func (c *httpClient) IsReady() (bool, error) { // TODO maybe don't need this after all
	pathURL, err := url.Parse("/api/v1/clienturl")
	if err != nil {
		return false, err
	}
	requestURL := c.config.ServerURL.ResolveReference(pathURL)
	resp, err := c.client.Get(requestURL.String())
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK, nil
}

func (c *httpClient) Close() error {
	return nil // TODO
}

func (c *httpClient) httpGet(apiPath string, result interface{}) error {
	return c.sendRequest(http.MethodGet, apiPath, nil, nil, result)
}

func (c *httpClient) httpPatch(apiPath string, body string, result interface{}) error {
	contentType := "text/plain"
	return c.sendRequest(http.MethodPatch, apiPath, strings.NewReader(body),
		&contentType, result)
}

func (c *httpClient) httpPostForm(apiPath string, formData url.Values, result interface{}) error {
	contentType := "application/x-www-form-urlencoded"
	return c.sendRequest(http.MethodPost, apiPath, strings.NewReader(formData.Encode()),
		&contentType, result)
}

func (c *httpClient) httpDelete(apiPath string, result interface{}) error {
	return c.sendRequest(http.MethodDelete, apiPath, nil, nil, result)
}

func (c *httpClient) sendRequest(method string, apiPath string, body io.Reader, contentType *string,
	result interface{}) error {
	// Resolve API path with server URL
	pathURL, err := url.Parse(apiPath)
	if err != nil {
		return err
	}
	requestURL := c.config.ServerURL.ResolveReference(pathURL)
	httpLogger := log.WithValues("method", method, "url", requestURL)

	// Create request and add authorization header(s)
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
		err := fmt.Errorf("server returned status: %s", resp.Status)
		errMsg, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			httpLogger.Error(err, "failed to read error message from response body")
		}
		httpLogger.Error(err, "request failed", "message", string(errMsg))
		return err
	}
	httpLogger.Info("request succeeded")

	// Decode response body stream directly
	return decodeResponse(resp.Body, result, httpLogger) // TODO maybe use Accept header to specify what kind of response we want? But what about errors then?
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
			dec := json.NewDecoder(body)
			err := dec.Decode(result)
			if err != nil {
				httpLogger.Error(err, "could not parse JSON response")
				buf, err := ioutil.ReadAll(dec.Buffered()) // XXX
				if err != nil {
					httpLogger.Error(err, "could not parse plain text response")
					return err
				}
				httpLogger.Info("got plain text response instead of JSON", "body", string(buf))
				return err
			}
			httpDebug.Info("parsed JSON response", "result", result)
		}
	}
	return nil
}

func apiPath(resource string, name *string, target *TargetAddress) string { // TODO maybe turn this into a type
	if target != nil {
		if name != nil {
			return fmt.Sprintf("/api/v1/targets/%s/%s/%s", url.PathEscape(target.String()), resource, *name)
		}
		return fmt.Sprintf("/api/v1/targets/%s/%s", url.PathEscape(target.String()), resource)
	}
	if name != nil {
		return fmt.Sprintf("/api/v1/%s/%s", resource, *name)
	}
	return fmt.Sprintf("/api/v1/%s", resource)
}
