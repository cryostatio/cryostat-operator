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

package containerjfr_test

import (
	"context"
	"net/http"

	certv1 "github.com/jetstack/cert-manager/pkg/apis/certmanager/v1"
	certMeta "github.com/jetstack/cert-manager/pkg/apis/meta/v1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/operator-framework/operator-sdk/pkg/log/zap"
	rhjmcv1beta1 "github.com/rh-jmc-team/container-jfr-operator/pkg/apis/rhjmc/v1beta1"
	"github.com/rh-jmc-team/container-jfr-operator/pkg/controller/containerjfr"
	"github.com/rh-jmc-team/container-jfr-operator/test"
)

var _ = Describe("ContainerjfrController", func() {
	var (
		objs       []runtime.Object
		handlers   []http.HandlerFunc
		server     *test.ContainerJFRServer
		client     client.Client
		controller *containerjfr.ReconcileContainerJFR
	)

	JustBeforeEach(func() {
		logf.SetLogger(zap.Logger())
		s := test.NewTestScheme()

		client = fake.NewFakeClientWithScheme(s, objs...)
		server = test.NewServer(client, handlers)
		controller = &containerjfr.ReconcileContainerJFR{
			Client:        client,
			Scheme:        s,
			ReconcilerTLS: test.NewTestReconcilerTLS(client),
		}
	})

	JustAfterEach(func() {
		server.VerifyRequestsReceived(handlers)
		server.Close()
	})

	BeforeEach(func() {
		objs = []runtime.Object{
			test.NewContainerJFR(), test.NewCACert(), test.NewTargetPod(),
			test.NewContainerJFRService(), test.NewJMXAuthSecret(),
		}
	})

	AfterEach(func() {
		objs = nil
	})

	Describe("reconciling a request", func() {
		Context("after Containerjfr already reconciled successfully", func() {
			It("should be idempotent", func() {
				req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "containerjfr", Namespace: "default"}}
				result, err := controller.Reconcile(req)
				Expect(err).ToNot(HaveOccurred())
				//Expect(result).To(Equal(reconcile.Result{}))

				obj := &rhjmcv1beta1.ContainerJFR{}
				err = client.Get(context.Background(), req.NamespacedName, obj)
				Expect(err).ToNot(HaveOccurred())

				caCert := &certv1.Certificate{}
				err = client.Get(context.Background(), types.NamespacedName{Name: "containerjfr-ca", Namespace: "default"}, caCert)
				Expect(err).ToNot(HaveOccurred())
				caCert.Status.Conditions = append(caCert.Status.Conditions, certv1.CertificateCondition{
					Type:   certv1.CertificateConditionReady,
					Status: certMeta.ConditionTrue,
				})
				err = client.Status().Update(context.Background(), caCert)
				Expect(err).ToNot(HaveOccurred())

				// Reconcile same containerjfr again
				result, err = controller.Reconcile(req)
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(Equal(reconcile.Result{}))

				obj2 := &rhjmcv1beta1.ContainerJFR{}
				err = client.Get(context.Background(), req.NamespacedName, obj2)
				Expect(err).ToNot(HaveOccurred())
				Expect(obj2.Status).To(Equal(obj.Status))
				Expect(obj2.Spec).To(Equal(obj.Spec))
			})
		})

	})
})
