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

	operatorv1beta1 "github.com/cryostatio/cryostat-operator/api/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
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
	config, err := ctrl.GetConfig()
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
