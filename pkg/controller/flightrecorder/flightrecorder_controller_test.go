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

package flightrecorder_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/operator-framework/operator-sdk/pkg/log/zap"
	rhjmcv1alpha1 "github.com/rh-jmc-team/container-jfr-operator/pkg/apis/rhjmc/v1alpha1"
	rhjmcv1alpha2 "github.com/rh-jmc-team/container-jfr-operator/pkg/apis/rhjmc/v1alpha2"
	"github.com/rh-jmc-team/container-jfr-operator/pkg/controller/flightrecorder"
	"github.com/rh-jmc-team/container-jfr-operator/test"
)

var _ = Describe("FlightRecorderController", func() {
	var (
		objs       []runtime.Object
		messages   []test.WsMessage
		server     *test.ContainerJFRServer
		client     client.Client
		controller *flightrecorder.ReconcileFlightRecorder
	)

	JustBeforeEach(func() {
		logf.SetLogger(zap.Logger())
		s := scheme.Scheme

		s.AddKnownTypes(rhjmcv1alpha1.SchemeGroupVersion, &rhjmcv1alpha1.ContainerJFR{},
			&rhjmcv1alpha1.ContainerJFRList{})
		s.AddKnownTypes(rhjmcv1alpha2.SchemeGroupVersion, &rhjmcv1alpha2.FlightRecorder{},
			&rhjmcv1alpha2.FlightRecorderList{})

		client = fake.NewFakeClientWithScheme(s, objs...)
		server = test.NewServer(client, messages)
		controller = &flightrecorder.ReconcileFlightRecorder{
			Client:     client,
			Scheme:     s,
			Reconciler: test.NewTestReconciler(client),
		}
	})

	JustAfterEach(func() {
		controller.CloseClient()
		server.Close()
	})

	BeforeEach(func() {
		objs = []runtime.Object{
			test.NewContainerJFR(), test.NewFlightRecorder(), test.NewTargetPod(), test.NewContainerJFRService(),
		}
		messages = []test.WsMessage{
			test.NewListEventTypesMessage(),
		}
	})

	AfterEach(func() {
		// Reset test inputs
		objs = nil
		messages = nil
	})

	Describe("reconciling a request", func() {
		Context("successfully updates FlightRecorder CR", func() {
			It("should update event type list", func() {
				req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "test-pod", Namespace: "default"}}
				result, err := controller.Reconcile(req)
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(Equal(reconcile.Result{}))

				obj := &rhjmcv1alpha2.FlightRecorder{}
				err = client.Get(context.Background(), req.NamespacedName, obj)
				Expect(err).ToNot(HaveOccurred())
				Expect(obj.Status.Events).To(Equal(messages[0].Reply.Payload))
			})
		})
		Context("after FlightRecorder already reconciled successfully", func() {
			BeforeEach(func() {
				messages = []test.WsMessage{
					test.NewListEventTypesMessage(),
					test.NewListEventTypesMessage(),
				}
			})
			It("should be idempotent", func() {
				req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "test-pod", Namespace: "default"}}
				result, err := controller.Reconcile(req)
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(Equal(reconcile.Result{}))

				obj := &rhjmcv1alpha2.FlightRecorder{}
				err = client.Get(context.Background(), req.NamespacedName, obj)
				Expect(err).ToNot(HaveOccurred())

				// Reconcile same FlightRecorder again
				result, err = controller.Reconcile(req)
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(Equal(reconcile.Result{}))

				obj2 := &rhjmcv1alpha2.FlightRecorder{}
				err = client.Get(context.Background(), req.NamespacedName, obj2)
				Expect(err).ToNot(HaveOccurred())
				Expect(obj2.Status).To(Equal(obj.Status))
				Expect(obj2.Spec).To(Equal(obj.Spec))
			})
		})
		Context("FlightRecorder does not exist", func() {
			BeforeEach(func() {
				messages = []test.WsMessage{}
			})
			It("should do nothing", func() {
				req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "does-not-exist", Namespace: "default"}}
				result, err := controller.Reconcile(req)
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(Equal(reconcile.Result{}))
			})
		})
		Context("FlightRecorder Status not updated yet", func() {
			BeforeEach(func() {
				otherFr := test.NewFlightRecorder()
				otherFr.Status = rhjmcv1alpha2.FlightRecorderStatus{}
				objs = []runtime.Object{
					test.NewContainerJFR(), otherFr, test.NewTargetPod(), test.NewContainerJFRService(),
				}
				messages = []test.WsMessage{}
			})
			It("should requeue", func() {
				req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "test-pod", Namespace: "default"}}
				result, err := controller.Reconcile(req)
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(Equal(reconcile.Result{RequeueAfter: time.Second}))
			})
		})
		Context("list-event-types command fails", func() {
			BeforeEach(func() {
				messages = []test.WsMessage{
					test.NewListEventTypesMessageFail(),
				}
			})
			It("should requeue with error", func() {
				expectReconcileError(controller)
			})
			It("should close Container JFR client", func() {
				req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "test-pod", Namespace: "default"}}
				controller.Reconcile(req)
				Expect(controller.IsClientConnected()).To(BeFalse())
			})
		})
		Context("Container JFR CR is missing", func() {
			BeforeEach(func() {
				objs = []runtime.Object{
					test.NewFlightRecorder(), test.NewTargetPod(), test.NewContainerJFRService(),
				}
				messages = []test.WsMessage{}
			})
			It("should requeue with error", func() {
				expectReconcileError(controller)
			})
		})
		Context("Container JFR service is missing", func() {
			BeforeEach(func() {
				objs = []runtime.Object{
					test.NewContainerJFR(), test.NewFlightRecorder(), test.NewTargetPod(),
				}
				messages = []test.WsMessage{}
			})
			It("should requeue with error", func() {
				expectReconcileError(controller)
			})
		})
		Context("Target pod is missing", func() {
			BeforeEach(func() {
				objs = []runtime.Object{
					test.NewContainerJFR(), test.NewFlightRecorder(), test.NewContainerJFRService(),
				}
				messages = []test.WsMessage{}
			})
			It("should requeue with error", func() {
				expectReconcileError(controller)
			})
		})
		Context("Target pod has no IP", func() {
			BeforeEach(func() {
				otherPod := test.NewTargetPod()
				otherPod.Status.PodIP = ""
				objs = []runtime.Object{
					test.NewContainerJFR(), test.NewFlightRecorder(), otherPod, test.NewContainerJFRService(),
				}
				messages = []test.WsMessage{}
			})
			It("should requeue with error", func() {
				expectReconcileError(controller)
			})
		})
	})
})

func expectReconcileError(controller *flightrecorder.ReconcileFlightRecorder) {
	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "test-pod", Namespace: "default"}}
	result, err := controller.Reconcile(req)
	Expect(err).To(HaveOccurred())
	Expect(result).To(Equal(reconcile.Result{}))
}
