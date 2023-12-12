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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	operatorv1beta1 "github.com/cryostatio/cryostat-operator/api/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"

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
	if err := operatorv1beta1.AddToScheme(scheme); err != nil {
		return nil, err
	}
	return newRESTClientForGV(config, scheme, &operatorv1beta1.GroupVersion)
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
func (c *CryostatClient) Get(ctx context.Context, name string) (*operatorv1beta1.Cryostat, error) {
	return get(ctx, c.restClient, c.resource, c.namespace, name, &operatorv1beta1.Cryostat{})
}

// Create creates the provided Cryostat CR
func (c *CryostatClient) Create(ctx context.Context, obj *operatorv1beta1.Cryostat) (*operatorv1beta1.Cryostat, error) {
	return create(ctx, c.restClient, c.resource, c.namespace, obj, &operatorv1beta1.Cryostat{})
}

// Update updates the provided Cryostat CR
func (c *CryostatClient) Update(ctx context.Context, obj *operatorv1beta1.Cryostat) (*operatorv1beta1.Cryostat, error) {
	return update(ctx, c.restClient, c.resource, c.namespace, obj, &operatorv1beta1.Cryostat{})
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

func update[r runtime.Object](ctx context.Context, c rest.Interface, res string, ns string, obj r, result r) (r, error) {
	err := c.Put().
		Namespace(ns).Resource(res).
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
	// Application URL pointing to Cryostat
	TargetClient *TargetClient
}

func (cs *CryostatRESTClientset) Targets() *TargetClient {
	return cs.TargetClient
}

func NewCryostatRESTClientset(applicationURL string) (*CryostatRESTClientset, error) {
	base, err := url.Parse(applicationURL)
	if err != nil {
		return nil, err
	}
	return &CryostatRESTClientset{
		TargetClient: &TargetClient{
			Base: base,
		},
	}, nil
}

// Client for Cryostat Target resources
type TargetClient struct {
	Base *url.URL
}

func (client *TargetClient) List() ([]Target, error) {
	return nil, nil
}

// Client for Cryostat Recording resources
type RecordingClient struct {
	Base *url.URL
}

func (client *RecordingClient) Get(ctx context.Context, recordingName string) ([]Target, error) {
	return nil, nil
}

func (client *RecordingClient) Create(ctx context.Context, options *RecordingCreateOptions) (*Recording, error) {
	req, err := NewCryostatRESTReqruest(http.MethodPost, options)
	if err != nil {
		return nil, fmt.Errorf("failed to create a Cryostat REST request: %s", err.Error())
	}

	r := &Recording{}
	err = req.Do(r)
	if err != nil {
		return nil, fmt.Errorf("failed to create recording: %s", err.Error())
	}
	return r, nil
}

type CryostatAPIResources interface {
	Target | Recording
}

type Target struct {
	ConnectUrl string `json:"connectUrl"`
	Alias      string `json:"alias,omitempty"`
}

type RecordingCreateOptions struct {
	RecordingName string `json:"recordingName"`
	Events        string `json:"events"`
	Duration      int32  `json:"duration,omitempty"`
	ToDisk        bool   `json:"toDisk,omitempty"`
	MaxSize       int32  `json:"maxSize,omitempty"`
	MaxAge        int32  `json:"maxAge,omitempty"`
}

type Recording struct {
	DownloadURL string `json:"downloadUrl"`
	ReportURL   string `json:"reportUrl"`
	Id          string `json:"id"`
	Name        string `json:"name"`
	StartTime   int32  `json:"startTime"`
	Duration    int32  `json:"duration"`
	Continuous  bool   `json:"continuous"`
	ToDisk      bool   `json:"toDisk"`
	MaxSize     int32  `json:"maxSize"`
	MaxAge      int32  `json:"maxAge"`
}

// CryostatRESTRequest
type CryostatRESTRequest struct {
	URL       *url.URL
	Verb      string
	Headers   http.Header
	Params    url.Values
	Body      io.Reader
	OpenShift bool
}

func (r *CryostatRESTRequest) Do(result any) error {
	// Construct a complete URL with params
	query := r.URL.Query()
	for key, values := range r.Params {
		for _, value := range values {
			query.Add(key, value)
		}
	}
	r.URL.RawQuery = query.Encode()

	// Add Auth Header
	err := r.SetAuthHeader()
	if err != nil {
		return err
	}

	request, err := http.NewRequest(r.Verb, r.URL.RequestURI(), r.Body)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(request)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	bodyAsBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %s", err.Error())
	}

	err = json.Unmarshal(bodyAsBytes, result)
	if err != nil {
		return fmt.Errorf("failed to JSON decode response body: %s", err.Error())
	}

	return nil
}

func (r *CryostatRESTRequest) SetAuthHeader() error {
	header := r.Headers
	// Authentication is only enabled on OCP (currently)
	if r.OpenShift {
		config, err := rest.InClusterConfig()
		if err != nil {
			return fmt.Errorf("failed to get in cluster config: %s", err.Error())
		}
		header.Add("Authorization", fmt.Sprintf("Bearer %s", config.BearerToken))
	}
	return nil
}

func NewCryostatRESTReqruest(verb string, body any) (*CryostatRESTRequest, error) {
	_body, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to JSON encode recording option: %s", err.Error())
	}
	req := &CryostatRESTRequest{
		Verb: http.MethodPost,
		Body: bytes.NewReader(_body),
	}
	return req, nil
}
