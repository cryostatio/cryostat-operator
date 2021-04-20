// Copyright The Cryostat Authors
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

package controllers_test

import (
	"context"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	operatorv1beta1 "github.com/cryostatio/cryostat-operator/api/v1beta1"
	"github.com/cryostatio/cryostat-operator/controllers"
	cryostatClient "github.com/cryostatio/cryostat-operator/controllers/client"
	"github.com/cryostatio/cryostat-operator/test"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

type recordingTestInput struct {
	controller *controllers.RecordingReconciler
	objs       []runtime.Object
	handlers   []http.HandlerFunc
	test.TestReconcilerConfig
}

var _ = Describe("RecordingController", func() {
	var t *recordingTestInput

	JustBeforeEach(func() {
		logger := zap.New()
		logf.SetLogger(logger)
		s := test.NewTestScheme()

		t.Client = fake.NewFakeClientWithScheme(s, t.objs...)
		t.Server = test.NewServer(t.Client, t.handlers, t.DisableTLS)
		t.controller = &controllers.RecordingReconciler{
			Client:     t.Client,
			Scheme:     s,
			Log:        logger,
			Reconciler: test.NewTestReconciler(&t.TestReconcilerConfig),
		}
	})

	JustAfterEach(func() {
		t.Server.VerifyRequestsReceived(t.handlers)
		t.Server.Close()
	})

	BeforeEach(func() {
		t = &recordingTestInput{
			objs: []runtime.Object{
				test.NewCryostat(), test.NewCACert(), test.NewFlightRecorder(),
				test.NewTargetPod(), test.NewCryostatService(), test.NewJMXAuthSecret(),
			},
		}
	})

	AfterEach(func() {
		// Reset test inputs
		t = nil
	})

	Describe("reconciling a request", func() {
		Context("with a new recording", func() {
			BeforeEach(func() {
				t.objs = append(t.objs, test.NewRecording())
				t.handlers = []http.HandlerFunc{
					test.NewDumpHandler(),
					test.NewListHandler(test.NewRecordingDescriptors("RUNNING", 30000)),
				}
			})
			It("updates status with recording info", func() {
				desc := test.NewRecordingDescriptors("RUNNING", 30000)[0]
				t.expectRecordingUpdated(&desc)
			})
			It("adds finalizer to recording", func() {
				t.expectRecordingFinalizerPresent()
			})
			It("should requeue after 10 seconds", func() {
				t.expectRecordingResult(reconcile.Result{RequeueAfter: 10 * time.Second})
			})
		})
		Context("with a new recording that fails", func() {
			BeforeEach(func() {
				t.objs = append(t.objs, test.NewRecording())
				t.handlers = []http.HandlerFunc{
					test.NewDumpFailHandler(),
				}
			})
			It("should requeue with error", func() {
				t.expectRecordingReconcileError()
			})
		})
		Context("with a new continuous recording", func() {
			BeforeEach(func() {
				t.objs = append(t.objs, test.NewContinuousRecording())
				t.handlers = []http.HandlerFunc{
					test.NewStartHandler(),
					test.NewListHandler(test.NewRecordingDescriptors("RUNNING", 0)),
				}
			})
			It("updates status with recording info", func() {
				desc := test.NewRecordingDescriptors("RUNNING", 0)[0]
				t.expectRecordingUpdated(&desc)
			})
			It("should requeue after 10 seconds", func() {
				t.expectRecordingResult(reconcile.Result{RequeueAfter: 10 * time.Second})
			})
		})
		Context("with a new continuous recording that fails", func() {
			BeforeEach(func() {
				t.objs = append(t.objs, test.NewContinuousRecording())
				t.handlers = []http.HandlerFunc{
					test.NewStartFailHandler(),
				}
			})
			It("should requeue with error", func() {
				t.expectRecordingReconcileError()
			})
		})
		Context("with a running recording", func() {
			BeforeEach(func() {
				t.objs = append(t.objs, test.NewRunningRecording())
				t.handlers = []http.HandlerFunc{
					test.NewListHandler(test.NewRecordingDescriptors("RUNNING", 30000)),
				}
			})
			It("should not change status", func() {
				t.expectRecordingStatusUnchaged()
			})
			It("should requeue after 10 seconds", func() {
				t.expectRecordingResult(reconcile.Result{RequeueAfter: 10 * time.Second})
			})
		})
		Context("with a running recording not found in Cryostat", func() {
			BeforeEach(func() {
				t.objs = append(t.objs, test.NewRunningRecording())
				t.handlers = []http.HandlerFunc{
					test.NewListHandler([]cryostatClient.RecordingDescriptor{}),
				}
			})
			It("should not change status", func() {
				t.expectRecordingStatusUnchaged()
			})
			It("should requeue after 10 seconds", func() {
				t.expectRecordingResult(reconcile.Result{RequeueAfter: 10 * time.Second})
			})
		})
		Context("when listing recordings fail", func() {
			BeforeEach(func() {
				t.objs = append(t.objs, test.NewRunningRecording())
				t.handlers = []http.HandlerFunc{
					test.NewListFailHandler(test.NewRecordingDescriptors("RUNNING", 30000)),
				}
			})
			It("should requeue with error", func() {
				t.expectRecordingReconcileError()
			})
		})
		Context("when listing recordings has unexpected state", func() {
			BeforeEach(func() {
				t.objs = append(t.objs, test.NewRunningRecording())
				t.handlers = []http.HandlerFunc{
					test.NewListHandler(test.NewRecordingDescriptors("DOES-NOT-EXIST", 30000)),
				}
			})
			It("should requeue with error", func() {
				t.expectRecordingReconcileError()
			})
		})
		Context("with a running recording to be stopped", func() {
			BeforeEach(func() {
				t.objs = append(t.objs, test.NewRecordingToStop())
				t.handlers = []http.HandlerFunc{
					test.NewStopHandler(),
					test.NewListHandler(test.NewRecordingDescriptors("STOPPED", 0)),
				}
			})
			It("should stop recording", func() {
				desc := test.NewRecordingDescriptors("STOPPED", 0)[0]
				t.expectRecordingUpdated(&desc)
			})
			It("should not requeue", func() {
				t.expectRecordingResult(reconcile.Result{})
			})
		})
		Context("with a running recording to be stopped that fails", func() {
			BeforeEach(func() {
				t.objs = append(t.objs, test.NewRecordingToStop())
				t.handlers = []http.HandlerFunc{
					test.NewStopFailHandler(),
				}
			})
			It("should requeue with error", func() {
				t.expectRecordingReconcileError()
			})
		})
		Context("with a stopped recording to be archived", func() {
			BeforeEach(func() {
				t.objs = append(t.objs, test.NewStoppedRecordingToArchive())
				t.handlers = []http.HandlerFunc{
					test.NewListHandler(test.NewRecordingDescriptors("STOPPED", 30000)),
					test.NewListSavedHandler([]cryostatClient.SavedRecording{}),
					test.NewSaveHandler(),
					test.NewListSavedHandler(test.NewSavedRecordings()),
				}
			})
			It("should update download URL", func() {
				req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "my-recording", Namespace: "default"}}

				before := &operatorv1beta1.Recording{}
				err := t.Client.Get(context.Background(), req.NamespacedName, before)
				Expect(err).ToNot(HaveOccurred())

				_, err = t.controller.Reconcile(context.Background(), req)
				Expect(err).ToNot(HaveOccurred())

				after := &operatorv1beta1.Recording{}
				err = t.Client.Get(context.Background(), req.NamespacedName, after)
				Expect(err).ToNot(HaveOccurred())

				// Should all be the same except for Download URL
				saved := test.NewSavedRecordings()[0]
				Expect(after.Status.State).To(Equal(before.Status.State))
				Expect(after.Status.Duration).To(Equal(before.Status.Duration))
				Expect(after.Status.StartTime).To(Equal(before.Status.StartTime))
				Expect(after.Status.DownloadURL).ToNot(BeNil())
				Expect(*after.Status.DownloadURL).To(Equal(saved.DownloadURL))
				Expect(after.Status.ReportURL).ToNot(BeNil())
				Expect(*after.Status.ReportURL).To(Equal(saved.ReportURL))
			})
			It("should not requeue", func() {
				t.expectRecordingResult(reconcile.Result{})
			})
		})
		Context("when listing saved recordings fails", func() {
			BeforeEach(func() {
				t.objs = append(t.objs, test.NewStoppedRecordingToArchive())
				t.handlers = []http.HandlerFunc{
					test.NewListHandler(test.NewRecordingDescriptors("STOPPED", 30000)),
					test.NewListSavedFailHandler(test.NewSavedRecordings()),
				}
			})
			It("should requeue with error", func() {
				t.expectRecordingReconcileError()
			})
		})
		Context("when archiving recording fails", func() {
			BeforeEach(func() {
				t.objs = append(t.objs, test.NewStoppedRecordingToArchive())
				t.handlers = []http.HandlerFunc{
					test.NewListHandler(test.NewRecordingDescriptors("STOPPED", 30000)),
					test.NewListSavedHandler(test.NewSavedRecordings()),
					test.NewSaveFailHandler(),
				}
			})
			It("should requeue with error", func() {
				t.expectRecordingReconcileError()
			})
		})
		Context("with a running recording to be stopped and archived", func() {
			BeforeEach(func() {
				t.objs = append(t.objs, test.NewRecordingToStopAndArchive())
				t.handlers = []http.HandlerFunc{
					test.NewStopHandler(),
					test.NewListHandler(test.NewRecordingDescriptors("STOPPED", 30000)),
					test.NewListSavedHandler([]cryostatClient.SavedRecording{}),
					test.NewSaveHandler(),
					test.NewListSavedHandler(test.NewSavedRecordings()),
				}
			})
			It("should stop recording", func() {
				req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "my-recording", Namespace: "default"}}
				_, err := t.controller.Reconcile(context.Background(), req)
				Expect(err).ToNot(HaveOccurred())

				obj := &operatorv1beta1.Recording{}
				err = t.Client.Get(context.Background(), req.NamespacedName, obj)
				Expect(err).ToNot(HaveOccurred())

				Expect(obj.Status.State).ToNot(BeNil())
				Expect(*obj.Status.State).To(Equal(operatorv1beta1.RecordingStateStopped))
			})
			It("should update download URL", func() {
				req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "my-recording", Namespace: "default"}}
				_, err := t.controller.Reconcile(context.Background(), req)
				Expect(err).ToNot(HaveOccurred())

				obj := &operatorv1beta1.Recording{}
				err = t.Client.Get(context.Background(), req.NamespacedName, obj)
				Expect(err).ToNot(HaveOccurred())

				Expect(obj.Status.DownloadURL).ToNot(BeNil())
				Expect(*obj.Status.DownloadURL).To(Equal("http://path/to/saved-test-recording.jfr"))
			})
			It("should update report URL", func() {
				req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "my-recording", Namespace: "default"}}
				_, err := t.controller.Reconcile(context.Background(), req)
				Expect(err).ToNot(HaveOccurred())

				obj := &operatorv1beta1.Recording{}
				err = t.Client.Get(context.Background(), req.NamespacedName, obj)
				Expect(err).ToNot(HaveOccurred())

				Expect(obj.Status.ReportURL).ToNot(BeNil())
				Expect(*obj.Status.ReportURL).To(Equal("http://path/to/saved-test-recording.html"))
			})
			It("should not requeue", func() {
				t.expectRecordingResult(reconcile.Result{})
			})
		})
		Context("with an archived recording", func() {
			BeforeEach(func() {
				t.objs = append(t.objs, test.NewArchivedRecording())
				t.handlers = []http.HandlerFunc{
					test.NewListHandler(test.NewRecordingDescriptors("STOPPED", 30000)),
					test.NewListSavedHandler(test.NewSavedRecordings()),
				}
			})
			It("should not change status", func() {
				t.expectRecordingStatusUnchaged()
			})
			It("should not requeue", func() {
				t.expectRecordingResult(reconcile.Result{})
			})
		})
		Context("with a deleted archived recording", func() {
			BeforeEach(func() {
				t.objs = append(t.objs, test.NewDeletedArchivedRecording())
				t.handlers = []http.HandlerFunc{
					test.NewListSavedHandler(test.NewSavedRecordings()),
					test.NewDeleteSavedHandler(),
					test.NewListHandler(test.NewRecordingDescriptors("STOPPED", 30000)),
					test.NewDeleteHandler(),
				}
			})
			It("should remove the finalizer", func() {
				t.expectRecordingFinalizerAbsent()
			})
			It("should not requeue", func() {
				t.expectRecordingResult(reconcile.Result{})
			})
		})
		Context("with a deleted recording with missing FlightRecorder", func() {
			BeforeEach(func() {
				t.objs = []runtime.Object{
					test.NewCryostat(), test.NewCACert(), test.NewTargetPod(),
					test.NewCryostatService(), test.NewDeletedArchivedRecording(),
					test.NewJMXAuthSecret(),
				}
				t.handlers = []http.HandlerFunc{
					test.NewListSavedNoJMXAuthHandler(test.NewSavedRecordings()),
					test.NewDeleteSavedNoJMXAuthHandler(),
				}
			})
			It("should remove the finalizer", func() {
				t.expectRecordingFinalizerAbsent()
			})
			It("should not requeue", func() {
				t.expectRecordingResult(reconcile.Result{})
			})
		})
		Context("when deleting the saved recording fails", func() {
			BeforeEach(func() {
				t.objs = append(t.objs, test.NewDeletedArchivedRecording())
				t.handlers = []http.HandlerFunc{
					test.NewListSavedHandler(test.NewSavedRecordings()),
					test.NewDeleteSavedFailHandler(),
				}
			})
			It("should keep the finalizer", func() {
				t.expectRecordingFinalizerPresent()
			})
			It("should requeue with error", func() {
				t.expectRecordingReconcileError()
			})
		})
		Context("when deleting the in-memory recording fails", func() {
			BeforeEach(func() {
				t.objs = append(t.objs, test.NewDeletedArchivedRecording())
				t.handlers = []http.HandlerFunc{
					test.NewListSavedHandler(test.NewSavedRecordings()),
					test.NewDeleteSavedHandler(),
					test.NewListHandler(test.NewRecordingDescriptors("STOPPED", 30000)),
					test.NewDeleteFailHandler(),
				}
			})
			It("should keep the finalizer", func() {
				t.expectRecordingFinalizerPresent()
			})
			It("should requeue with error", func() {
				t.expectRecordingReconcileError()
			})
		})
		Context("Recording does not exist", func() {
			BeforeEach(func() {
				t.handlers = []http.HandlerFunc{}
			})
			It("should do nothing", func() {
				req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "does-not-exist", Namespace: "default"}}
				result, err := t.controller.Reconcile(context.Background(), req)
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(Equal(reconcile.Result{}))
			})
		})
		Context("FlightRecorder Status not updated yet", func() {
			BeforeEach(func() {
				otherFr := test.NewFlightRecorder()
				otherFr.Status = operatorv1beta1.FlightRecorderStatus{}
				t.objs = []runtime.Object{
					test.NewCryostat(), test.NewCACert(), otherFr, test.NewTargetPod(),
					test.NewCryostatService(), test.NewRecording(), test.NewJMXAuthSecret(),
				}
				t.handlers = []http.HandlerFunc{}
			})
			It("should requeue", func() {
				t.expectRecordingResult(reconcile.Result{RequeueAfter: time.Second})
			})
		})
		Context("Cryostat CR is missing", func() {
			BeforeEach(func() {
				t.objs = []runtime.Object{
					test.NewFlightRecorder(), test.NewCACert(), test.NewTargetPod(),
					test.NewCryostatService(), test.NewRecording(), test.NewJMXAuthSecret(),
				}
				t.handlers = []http.HandlerFunc{}
			})
			It("should requeue with error", func() {
				t.expectRecordingReconcileError()
			})
		})
		Context("Cryostat service is missing", func() {
			BeforeEach(func() {
				t.objs = []runtime.Object{
					test.NewCryostat(), test.NewCACert(), test.NewFlightRecorder(),
					test.NewTargetPod(), test.NewRecording(), test.NewJMXAuthSecret(),
				}
				t.handlers = []http.HandlerFunc{}
			})
			It("should requeue with error", func() {
				t.expectRecordingReconcileError()
			})
		})
		Context("FlightRecorder is missing", func() {
			BeforeEach(func() {
				t.objs = []runtime.Object{
					test.NewCryostat(), test.NewCACert(), test.NewTargetPod(),
					test.NewCryostatService(), test.NewRecording(), test.NewJMXAuthSecret(),
				}
				t.handlers = []http.HandlerFunc{}
			})
			It("should not requeue", func() {
				req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "my-recording", Namespace: "default"}}
				result, err := t.controller.Reconcile(context.Background(), req)
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(Equal(reconcile.Result{}))
			})
			It("should set FlightRecorder label", func() {
				obj := t.reconcileRecordingAndGet()
				Expect(obj.Labels).To(HaveKeyWithValue(operatorv1beta1.RecordingLabel, "test-pod"))
			})
		})
		Context("FlightRecorder is not defined in Recording", func() {
			BeforeEach(func() {
				recording := test.NewRecording()
				recording.Spec.FlightRecorder = nil
				t.objs = []runtime.Object{
					test.NewCryostat(), test.NewCACert(), test.NewFlightRecorder(), test.NewTargetPod(),
					test.NewCryostatService(), recording, test.NewJMXAuthSecret(),
				}
				t.handlers = []http.HandlerFunc{}
			})
			It("should not requeue", func() {
				req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "my-recording", Namespace: "default"}}
				result, err := t.controller.Reconcile(context.Background(), req)
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(Equal(reconcile.Result{}))
			})
			It("should not set FlightRecorder label", func() {
				obj := t.reconcileRecordingAndGet()
				Expect(obj.Labels).To(BeEmpty())
			})
		})
		Context("Target pod is missing", func() {
			BeforeEach(func() {
				t.objs = []runtime.Object{
					test.NewCryostat(), test.NewCACert(), test.NewFlightRecorder(),
					test.NewCryostatService(), test.NewRecording(), test.NewJMXAuthSecret(),
				}
				t.handlers = []http.HandlerFunc{}
			})
			It("should requeue with error", func() {
				t.expectRecordingReconcileError()
			})
		})
		Context("Target pod has no IP", func() {
			BeforeEach(func() {
				otherPod := test.NewTargetPod()
				otherPod.Status.PodIP = ""
				t.objs = []runtime.Object{
					test.NewCryostat(), test.NewCACert(), test.NewFlightRecorder(), otherPod,
					test.NewCryostatService(), test.NewRecording(), test.NewJMXAuthSecret(),
				}
				t.handlers = []http.HandlerFunc{}
			})
			It("should requeue with error", func() {
				t.expectRecordingReconcileError()
			})
		})
	})
})

func (t *recordingTestInput) expectRecordingUpdated(desc *cryostatClient.RecordingDescriptor) {
	obj := t.reconcileRecordingAndGet()

	Expect(obj.Labels).To(HaveKeyWithValue(operatorv1beta1.RecordingLabel, "test-pod"))

	Expect(obj.Status.State).ToNot(BeNil())
	Expect(*obj.Status.State).To(Equal(operatorv1beta1.RecordingState(desc.State)))
	// Converted to RFC3339 during serialization (sub-second precision lost)
	expectedTime := metav1.Unix(0, desc.StartTime*int64(time.Millisecond)).Rfc3339Copy()
	Expect(obj.Status.State).ToNot(BeNil())
	Expect(obj.Status.StartTime.Equal(&expectedTime)).To(BeTrue())
	Expect(obj.Status.Duration).To(Equal(metav1.Duration{
		Duration: time.Duration(desc.Duration) * time.Millisecond,
	}))
	Expect(obj.Status.DownloadURL).ToNot(BeNil())
	Expect(*obj.Status.DownloadURL).To(Equal(desc.DownloadURL))
	Expect(obj.Status.ReportURL).ToNot(BeNil())
	Expect(*obj.Status.ReportURL).To(Equal(desc.ReportURL))
}

func (t *recordingTestInput) expectRecordingStatusUnchaged() {
	before := &operatorv1beta1.Recording{}
	err := t.Client.Get(context.Background(), types.NamespacedName{Name: "my-recording", Namespace: "default"}, before)
	Expect(err).ToNot(HaveOccurred())

	after := t.reconcileRecordingAndGet()
	Expect(after.Status).To(Equal(before.Status))
}

func (t *recordingTestInput) expectRecordingFinalizerPresent() {
	obj := t.reconcileRecordingAndGet()
	finalizers := obj.GetFinalizers()
	Expect(finalizers).To(ContainElement("operator.cryostat.io/recording.finalizer"))
}

func (t *recordingTestInput) expectRecordingFinalizerAbsent() {
	obj := t.reconcileRecordingAndGet()
	finalizers := obj.GetFinalizers()
	Expect(finalizers).ToNot(ContainElement("operator.cryostat.io/recording.finalizer"))
}

func (t *recordingTestInput) expectRecordingReconcileError() {
	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "my-recording", Namespace: "default"}}
	result, err := t.controller.Reconcile(context.Background(), req)
	Expect(err).To(HaveOccurred())
	Expect(result).To(Equal(reconcile.Result{}))
}

func (t *recordingTestInput) expectRecordingResult(result reconcile.Result) {
	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "my-recording", Namespace: "default"}}
	result, err := t.controller.Reconcile(context.Background(), req)
	Expect(err).ToNot(HaveOccurred())
	Expect(result).To(Equal(result))
}

func (t *recordingTestInput) reconcileRecordingAndGet() *operatorv1beta1.Recording {
	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "my-recording", Namespace: "default"}}
	t.controller.Reconcile(context.Background(), req)

	obj := &operatorv1beta1.Recording{}
	err := t.Client.Get(context.Background(), req.NamespacedName, obj)
	Expect(err).ToNot(HaveOccurred())
	return obj
}
