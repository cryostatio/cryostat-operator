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

package endpoints_test

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/operator-framework/operator-sdk/pkg/log/zap"
	rhjmcv1beta1 "github.com/rh-jmc-team/container-jfr-operator/pkg/apis/rhjmc/v1beta1"
	"github.com/rh-jmc-team/container-jfr-operator/pkg/controller/endpoints"
	"github.com/rh-jmc-team/container-jfr-operator/test"
)

var _ = Describe("EndpointsController", func() {
	var (
		objs       []runtime.Object
		client     client.Client
		controller *endpoints.ReconcileEndpoints
	)

	JustBeforeEach(func() {
		logf.SetLogger(zap.Logger())
		s := test.NewTestScheme()

		client = fake.NewFakeClientWithScheme(s, objs...)
		controller = &endpoints.ReconcileEndpoints{
			Client:     client,
			Scheme:     s,
			Reconciler: test.NewTestReconcilerNoServer(client),
		}
	})

	AfterEach(func() {
		objs = nil
	})

	Describe("reconciling a request", func() {
		Context("successfully reconcile", func() {
			BeforeEach(func() {
				objs = []runtime.Object{
					test.NewContainerJFR(), test.NewTestService(),
					test.NewTargetPod(), test.NewTestEndpoints(),
				}
			})
			It("should create new flightrecorder", func() {
				req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "test-svc", Namespace: "default"}}
				result, err := controller.Reconcile(req)
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(Equal(reconcile.Result{}))

				found := &rhjmcv1beta1.FlightRecorder{}
				err = client.Get(context.Background(), types.NamespacedName{Name: "test-pod", Namespace: "default"}, found)
				Expect(err).ToNot(HaveOccurred())
				// compare found to desired spec
				expected := test.NewFlightRecorderNoJMXAuth()
				Expect(found.TypeMeta).To(Equal(expected.TypeMeta))
				Expect(found.ObjectMeta.Name).To(Equal(expected.ObjectMeta.Name))
				Expect(found.ObjectMeta.Namespace).To(Equal(expected.ObjectMeta.Namespace))
				Expect(found.ObjectMeta.Labels).To(Equal(expected.ObjectMeta.Labels))
				Expect(found.ObjectMeta.OwnerReferences).To(Equal(expected.ObjectMeta.OwnerReferences))
				Expect(found.Spec).To(Equal(expected.Spec))
			})
		})
		Context("successfully reconcile container JFR", func() {
			BeforeEach(func() {
				objs = []runtime.Object{
					test.NewContainerJFR(), test.NewContainerJFRService(),
					test.NewContainerJFREndpoints(), test.NewContainerJFRPod(),
					test.NewJMXAuthSecretForCJFR(),
				}
			})
			It("should create new flightrecorder", func() {
				req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "containerjfr", Namespace: "default"}}
				result, err := controller.Reconcile(req)
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(Equal(reconcile.Result{}))

				found := &rhjmcv1beta1.FlightRecorder{}
				err = client.Get(context.Background(), types.NamespacedName{Name: "containerjfr-pod", Namespace: "default"}, found)
				Expect(err).ToNot(HaveOccurred())
				// compare found to desired spec
				expected := test.NewFlightRecorderForCJFR()

				compareFlightRecorders(found, expected)
			})
		})
		Context("endpoints does not exist", func() {
			BeforeEach(func() {
				objs = []runtime.Object{
					test.NewContainerJFR(),
				}
			})
			It("should return without error", func() {
				req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "test-svc", Namespace: "default"}}
				result, err := controller.Reconcile(req)
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(Equal(reconcile.Result{}))
			})
		})
		Context("endpoints has no targetRef", func() {
			BeforeEach(func() {
				objs = []runtime.Object{
					test.NewContainerJFR(), test.NewTestService(),
					test.NewTargetPod(), test.NewTestEndpointsNoTargetRef(),
				}
			})
			It("should return without error", func() {
				req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "test-svc", Namespace: "default"}}
				result, err := controller.Reconcile(req)
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(Equal(reconcile.Result{}))
			})
			It("should not create flightrecorder", func() {
				req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "test-svc", Namespace: "default"}}
				controller.Reconcile(req)
				recorder := &rhjmcv1beta1.FlightRecorder{}
				err := client.Get(context.Background(), types.NamespacedName{Name: "test-pod", Namespace: "default"}, recorder)
				Expect(kerrors.IsNotFound(err)).To(BeTrue())
			})
		})
		Context("endpoints has no ports", func() {
			BeforeEach(func() {
				objs = []runtime.Object{
					test.NewContainerJFR(), test.NewTestService(),
					test.NewTargetPod(), test.NewTestEndpointsNoPorts(),
				}
			})
			It("should return without error", func() {
				req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "test-svc", Namespace: "default"}}
				result, err := controller.Reconcile(req)
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(Equal(reconcile.Result{}))
			})
			It("should not create flightrecorder", func() {
				req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "test-svc", Namespace: "default"}}
				controller.Reconcile(req)
				recorder := &rhjmcv1beta1.FlightRecorder{}
				err := client.Get(context.Background(), types.NamespacedName{Name: "test-pod", Namespace: "default"}, recorder)
				Expect(kerrors.IsNotFound(err)).To(BeTrue())
			})
		})
		Context("endpoints only has default port", func() {
			BeforeEach(func() {
				objs = []runtime.Object{
					test.NewContainerJFR(), test.NewTestService(),
					test.NewTargetPod(), test.NewTestEndpointsNoJMXPort(),
				}
			})
			It("should return without error", func() {
				req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "test-svc", Namespace: "default"}}
				result, err := controller.Reconcile(req)
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(Equal(reconcile.Result{}))
			})
			It("should create flightrecorder", func() {
				req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "test-svc", Namespace: "default"}}
				controller.Reconcile(req)
				recorder := &rhjmcv1beta1.FlightRecorder{}
				err := client.Get(context.Background(), types.NamespacedName{Name: "test-pod", Namespace: "default"}, recorder)
				Expect(err).ToNot(HaveOccurred())
				expected := test.NewFlightRecorderNoJMXAuth()
				compareFlightRecorders(recorder, expected)
			})
		})

	})
})

func compareFlightRecorders(found *rhjmcv1beta1.FlightRecorder, expected *rhjmcv1beta1.FlightRecorder) {
	Expect(found.TypeMeta).To(Equal(expected.TypeMeta))
	Expect(found.ObjectMeta.Name).To(Equal(expected.ObjectMeta.Name))
	Expect(found.ObjectMeta.Namespace).To(Equal(expected.ObjectMeta.Namespace))
	Expect(found.ObjectMeta.Labels).To(Equal(expected.ObjectMeta.Labels))
	Expect(found.ObjectMeta.OwnerReferences).To(Equal(expected.ObjectMeta.OwnerReferences))
	Expect(found.Spec).To(Equal(expected.Spec))
}
