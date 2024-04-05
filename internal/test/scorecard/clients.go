// Copyright The Cryostat Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package scorecard

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	operatorv1beta2 "github.com/cryostatio/cryostat-operator/api/v1beta2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/util/wait"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// CryostatClientset is a Kubernetes Clientset that can also
// perform CRUD operations on Cryostat Operator CRs
type CryostatClientset struct {
	*kubernetes.Clientset
	operatorClient *OperatorCRDClient
}

// OperatorCRDs returns a OperatorCRDClient
func (c *CryostatClientset) OperatorCRDs() *OperatorCRDClient {
	return c.operatorClient
}

// NewClientset creates a CryostatClientset
func NewClientset() (*CryostatClientset, error) {
	// Get in-cluster REST config from pod
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}

	// Create a new Clientset to communicate with the cluster
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	// Add custom client for our CRDs
	client, err := newOperatorCRDClient(config)
	if err != nil {
		return nil, err
	}
	return &CryostatClientset{
		Clientset:      clientset,
		operatorClient: client,
	}, nil
}

// OperatorCRDClient is a Kubernetes REST client for performing operations on
// Cryostat Operator custom resources
type OperatorCRDClient struct {
	client *rest.RESTClient
}

// Cryostats returns a CryostatClient configured to a specific namespace
func (c *OperatorCRDClient) Cryostats(namespace string) *CryostatClient {
	return &CryostatClient{
		restClient: c.client,
		namespace:  namespace,
		resource:   "cryostats",
	}
}

func newOperatorCRDClient(config *rest.Config) (*OperatorCRDClient, error) {
	client, err := newCRDClient(config)
	if err != nil {
		return nil, err
	}
	return &OperatorCRDClient{
		client: client,
	}, nil
}

func newCRDClient(config *rest.Config) (*rest.RESTClient, error) {
	scheme := runtime.NewScheme()
	if err := operatorv1beta2.AddToScheme(scheme); err != nil {
		return nil, err
	}
	return newRESTClientForGV(config, scheme, &operatorv1beta2.GroupVersion)
}

func newRESTClientForGV(config *rest.Config, scheme *runtime.Scheme, gv *schema.GroupVersion) (*rest.RESTClient, error) {
	configCopy := *config
	configCopy.GroupVersion = gv
	configCopy.APIPath = "/apis"
	configCopy.ContentType = runtime.ContentTypeJSON
	configCopy.NegotiatedSerializer = serializer.WithoutConversionCodecFactory{CodecFactory: serializer.NewCodecFactory(scheme)}
	return rest.RESTClientFor(&configCopy)
}

// CryostatClient contains methods to perform operations on
// Cryostat custom resources
type CryostatClient struct {
	restClient rest.Interface
	namespace  string
	resource   string
}

// Get returns a Cryostat CR for the given name
func (c *CryostatClient) Get(ctx context.Context, name string) (*operatorv1beta2.Cryostat, error) {
	return get(ctx, c.restClient, c.resource, c.namespace, name, &operatorv1beta2.Cryostat{})
}

// Create creates the provided Cryostat CR
func (c *CryostatClient) Create(ctx context.Context, obj *operatorv1beta2.Cryostat) (*operatorv1beta2.Cryostat, error) {
	return create(ctx, c.restClient, c.resource, c.namespace, obj, &operatorv1beta2.Cryostat{})
}

// Update updates the provided Cryostat CR
func (c *CryostatClient) Update(ctx context.Context, obj *operatorv1beta2.Cryostat) (*operatorv1beta2.Cryostat, error) {
	return update(ctx, c.restClient, c.resource, c.namespace, obj, &operatorv1beta2.Cryostat{}, obj.Name)
}

// Delete deletes the Cryostat CR with the given name
func (c *CryostatClient) Delete(ctx context.Context, name string, options *metav1.DeleteOptions) error {
	return delete(ctx, c.restClient, c.resource, c.namespace, name, options)
}

func get[r runtime.Object](ctx context.Context, c rest.Interface, res string, ns string, name string, result r) (r, error) {
	err := c.Get().
		Namespace(ns).Resource(res).
		Name(name).Do(ctx).Into(result)
	return result, err
}

func create[r runtime.Object](ctx context.Context, c rest.Interface, res string, ns string, obj r, result r) (r, error) {
	err := c.Post().
		Namespace(ns).Resource(res).
		Body(obj).Do(ctx).Into(result)
	return result, err
}

func update[r runtime.Object](ctx context.Context, c rest.Interface, res string, ns string, obj r, result r, name string) (r, error) {
	err := c.Put().
		Namespace(ns).Resource(res).Name(name).
		Body(obj).Do(ctx).Into(result)
	return result, err
}

func delete(ctx context.Context, c rest.Interface, res string, ns string, name string, opts *metav1.DeleteOptions) error {
	return c.Delete().
		Namespace(ns).Resource(res).
		Name(name).Body(opts).Do(ctx).
		Error()
}

// CryostatRESTClientset contains methods to interact with
// the Cryostat API
type CryostatRESTClientset struct {
	TargetClient     *TargetClient
	RecordingClient  *RecordingClient
	CredentialClient *CredentialClient
}

func (c *CryostatRESTClientset) Targets() *TargetClient {
	return c.TargetClient
}

func (c *CryostatRESTClientset) Recordings() *RecordingClient {
	return c.RecordingClient
}

func (c *CryostatRESTClientset) Credential() *CredentialClient {
	return c.CredentialClient
}

func NewCryostatRESTClientset(base *url.URL) *CryostatRESTClientset {
	commonClient := &commonCryostatRESTClient{
		Base:   base,
		Client: NewHttpClient(),
	}

	return &CryostatRESTClientset{
		TargetClient: &TargetClient{
			commonCryostatRESTClient: commonClient,
		},
		RecordingClient: &RecordingClient{
			commonCryostatRESTClient: commonClient,
		},
		CredentialClient: &CredentialClient{
			commonCryostatRESTClient: commonClient,
		},
	}
}

type commonCryostatRESTClient struct {
	Base *url.URL
	*http.Client
}

// Client for Cryostat Target resources
type TargetClient struct {
	*commonCryostatRESTClient
}

func (client *TargetClient) List(ctx context.Context) ([]Target, error) {
	url := client.Base.JoinPath("/api/v1/targets")
	header := make(http.Header)
	header.Add("Accept", "*/*")

	resp, err := SendRequest(ctx, client.Client, http.MethodGet, url.String(), nil, header)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if !StatusOK(resp.StatusCode) {
		return nil, fmt.Errorf("API request failed with status code: %d, response body: %s, and headers:\n%s", resp.StatusCode, ReadError(resp), ReadHeader(resp))
	}

	targets := make([]Target, 0)
	err = ReadJSON(resp, &targets)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %s", err.Error())
	}

	return targets, nil
}

func (client *TargetClient) Create(ctx context.Context, options *Target) (*Target, error) {
	url := client.Base.JoinPath("/api/v2/targets")
	header := make(http.Header)
	header.Add("Content-Type", "application/x-www-form-urlencoded")
	header.Add("Accept", "*/*")
	body := options.ToFormData()

	resp, err := SendRequest(ctx, client.Client, http.MethodPost, url.String(), &body, header)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if !StatusOK(resp.StatusCode) {
		return nil, fmt.Errorf("API request failed with status code: %d, response body: %s, and headers:\n%s", resp.StatusCode, ReadError(resp), ReadHeader(resp))
	}

	targetResp := &CustomTargetResponse{}
	err = ReadJSON(resp, targetResp)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %s", err.Error())
	}

	return targetResp.Data.Result, nil
}

// Client for Cryostat Recording resources
type RecordingClient struct {
	*commonCryostatRESTClient
}

func (client *RecordingClient) List(ctx context.Context, connectUrl string) ([]Recording, error) {
	url := client.Base.JoinPath(fmt.Sprintf("/api/v1/targets/%s/recordings", url.PathEscape(connectUrl)))
	header := make(http.Header)
	header.Add("Accept", "*/*")

	resp, err := SendRequest(ctx, client.Client, http.MethodGet, url.String(), nil, header)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if !StatusOK(resp.StatusCode) {
		return nil, fmt.Errorf("API request failed with status code: %d, response body: %s, and headers:\n%s", resp.StatusCode, ReadError(resp), ReadHeader(resp))
	}

	recordings := make([]Recording, 0)
	err = ReadJSON(resp, &recordings)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %s", err.Error())
	}

	return recordings, nil
}

func (client *RecordingClient) Get(ctx context.Context, connectUrl string, recordingName string) (*Recording, error) {
	recordings, err := client.List(ctx, connectUrl)
	if err != nil {
		return nil, err
	}

	for _, rec := range recordings {
		if rec.Name == recordingName {
			return &rec, nil
		}
	}

	return nil, fmt.Errorf("recording %s does not exist for target %s", recordingName, connectUrl)
}

func (client *RecordingClient) Create(ctx context.Context, connectUrl string, options *RecordingCreateOptions) (*Recording, error) {
	url := client.Base.JoinPath(fmt.Sprintf("/api/v1/targets/%s/recordings", url.PathEscape(connectUrl)))
	body := options.ToFormData()
	header := make(http.Header)
	header.Add("Content-Type", "application/x-www-form-urlencoded")
	header.Add("Accept", "*/*")

	resp, err := SendRequest(ctx, client.Client, http.MethodPost, url.String(), &body, header)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if !StatusOK(resp.StatusCode) {
		return nil, fmt.Errorf("API request failed with status code: %d, response body: %s, and headers:\n%s", resp.StatusCode, ReadError(resp), ReadHeader(resp))
	}

	recording := &Recording{}
	err = ReadJSON(resp, recording)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %s", err.Error())
	}

	return recording, err
}

func (client *RecordingClient) Archive(ctx context.Context, connectUrl string, recordingName string) (string, error) {
	url := client.Base.JoinPath(fmt.Sprintf("/api/v1/targets/%s/recordings/%s", url.PathEscape(connectUrl), url.PathEscape(recordingName)))
	body := "SAVE"
	header := make(http.Header)
	header.Add("Content-Type", "text/plain")
	header.Add("Accept", "*/*")

	resp, err := SendRequest(ctx, client.Client, http.MethodPatch, url.String(), &body, header)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if !StatusOK(resp.StatusCode) {
		return "", fmt.Errorf("API request failed with status code: %d, response body: %s, and headers:\n%s", resp.StatusCode, ReadError(resp), ReadHeader(resp))
	}

	bodyAsString, err := ReadString(resp)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %s", err.Error())
	}

	return bodyAsString, nil
}

func (client *RecordingClient) Stop(ctx context.Context, connectUrl string, recordingName string) error {
	url := client.Base.JoinPath(fmt.Sprintf("/api/v1/targets/%s/recordings/%s", url.PathEscape(connectUrl), url.PathEscape(recordingName)))
	body := "STOP"
	header := make(http.Header)
	header.Add("Content-Type", "text/plain")
	header.Add("Accept", "*/*")

	resp, err := SendRequest(ctx, client.Client, http.MethodPatch, url.String(), &body, header)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if !StatusOK(resp.StatusCode) {
		return fmt.Errorf("API request failed with status code: %d, response body: %s, and headers:\n%s", resp.StatusCode, ReadError(resp), ReadHeader(resp))
	}

	return nil
}

func (client *RecordingClient) Delete(ctx context.Context, connectUrl string, recordingName string) error {
	url := client.Base.JoinPath(fmt.Sprintf("/api/v1/targets/%s/recordings/%s", url.PathEscape(connectUrl), url.PathEscape(recordingName)))
	header := make(http.Header)

	resp, err := SendRequest(ctx, client.Client, http.MethodDelete, url.String(), nil, header)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if !StatusOK(resp.StatusCode) {
		return fmt.Errorf("API request failed with status code: %d, response body: %s, and headers:\n%s", resp.StatusCode, ReadError(resp), ReadHeader(resp))
	}

	return nil
}

func (client *RecordingClient) GenerateReport(ctx context.Context, connectUrl string, recordingName *Recording) (map[string]interface{}, error) {
	reportURL := recordingName.ReportURL

	if len(reportURL) < 1 {
		return nil, fmt.Errorf("report URL is not available")
	}

	header := make(http.Header)
	header.Add("Accept", "application/json")

	resp, err := SendRequest(ctx, client.Client, http.MethodGet, reportURL, nil, header)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if !StatusOK(resp.StatusCode) {
		return nil, fmt.Errorf("API request failed with status code: %d, response body: %s, and headers:\n%s", resp.StatusCode, ReadError(resp), ReadHeader(resp))
	}

	report := make(map[string]interface{}, 0)
	err = ReadJSON(resp, &report)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %s", err.Error())
	}

	return report, nil
}

func (client *RecordingClient) ListArchives(ctx context.Context, connectUrl string) ([]Archive, error) {
	url := client.Base.JoinPath("/api/v2.2/graphql")

	query := &GraphQLQuery{
		Query: `
			query ArchivedRecordingsForTarget($connectUrl: String) {
				archivedRecordings(filter: { sourceTarget: $connectUrl }) {
					data {
						name
						downloadUrl
						reportUrl
						metadata {
						labels
						}
						size
					}
				}
			}
		`,
		Variables: map[string]string{
			connectUrl: connectUrl,
		},
	}
	queryJSON, err := query.ToJSON()
	if err != nil {
		return nil, fmt.Errorf("failed to construct graph query: %s", err.Error())
	}
	body := string(queryJSON)

	header := make(http.Header)
	header.Add("Content-Type", "application/json")
	header.Add("Accept", "*/*")

	resp, err := SendRequest(ctx, client.Client, http.MethodPost, url.String(), &body, header)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if !StatusOK(resp.StatusCode) {
		return nil, fmt.Errorf("API request failed with status code: %d, response body: %s, and headers:\n%s", resp.StatusCode, ReadError(resp), ReadHeader(resp))
	}

	graphQLResponse := &ArchiveGraphQLResponse{}
	err = ReadJSON(resp, graphQLResponse)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %s", err.Error())
	}

	return graphQLResponse.Data.ArchivedRecordings.Data, nil
}

type CredentialClient struct {
	*commonCryostatRESTClient
}

func (client *CredentialClient) Create(ctx context.Context, credential *Credential) error {
	url := client.Base.JoinPath("/api/v2.2/credentials")
	body := credential.ToFormData()
	header := make(http.Header)
	header.Add("Content-Type", "application/x-www-form-urlencoded")

	resp, err := SendRequest(ctx, client.Client, http.MethodPost, url.String(), &body, header)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if !StatusOK(resp.StatusCode) {
		return fmt.Errorf("API request failed with status code: %d, response body: %s, and headers:\n%s", resp.StatusCode, ReadError(resp), ReadHeader(resp))
	}

	return nil
}

func ReadJSON(resp *http.Response, result interface{}) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	err = json.Unmarshal(body, result)
	if err != nil {
		return err
	}
	return nil
}

func ReadString(resp *http.Response) (string, error) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func ReadHeader(resp *http.Response) string {
	header := ""
	for name, value := range resp.Header {
		for _, h := range value {
			header += fmt.Sprintf("%s: %s\n", name, h)
		}
	}
	return header
}

func ReadError(resp *http.Response) string {
	body, _ := ReadString(resp)
	return body
}

func NewHttpClient() *http.Client {
	client := &http.Client{
		Timeout: testTimeout,
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	// Ignore verifying certs
	transport.TLSClientConfig.InsecureSkipVerify = true

	client.Transport = transport
	return client
}

func NewHttpRequest(ctx context.Context, method string, url string, body *string, header http.Header) (*http.Request, error) {
	var reqBody io.Reader
	if body != nil {
		reqBody = strings.NewReader(*body)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return nil, err
	}
	if header != nil {
		req.Header = header
	}
	// Authentication is only enabled on OCP. Ignored on k8s.
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get in-cluster configurations: %s", err.Error())
	}
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", base64.StdEncoding.EncodeToString([]byte(config.BearerToken))))
	return req, nil
}

func StatusOK(statusCode int) bool {
	return statusCode >= 200 && statusCode < 300
}

func SendRequest(ctx context.Context, httpClient *http.Client, method string, url string, body *string, header http.Header) (*http.Response, error) {
	var response *http.Response
	err := wait.PollImmediateUntilWithContext(ctx, time.Second, func(ctx context.Context) (done bool, err error) {
		// Create a new request
		req, err := NewHttpRequest(ctx, method, url, body, header)
		if err != nil {
			return false, fmt.Errorf("failed to create an http request: %s", err.Error())
		}

		resp, err := httpClient.Do(req)
		if err != nil {
			// Retry when connection is closed.
			if errors.Is(err, io.EOF) {
				return false, nil
			}
			return false, err
		}
		response = resp
		return true, nil
	})

	return response, err
}
