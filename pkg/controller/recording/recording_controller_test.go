package recording_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	jfrclient "github.com/rh-jmc-team/container-jfr-operator/pkg/client"
	"github.com/rh-jmc-team/container-jfr-operator/pkg/controller/recording"
	"github.com/rh-jmc-team/container-jfr-operator/test"
)

var _ = Describe("RecordingController", func() {
	var (
		objs       []runtime.Object
		messages   []test.WsMessage
		server     *test.ContainerJFRServer
		client     client.Client
		controller *recording.ReconcileRecording
	)

	JustBeforeEach(func() {
		logf.SetLogger(zap.Logger())
		s := scheme.Scheme

		s.AddKnownTypes(rhjmcv1alpha1.SchemeGroupVersion, &rhjmcv1alpha1.ContainerJFR{},
			&rhjmcv1alpha1.ContainerJFRList{})
		s.AddKnownTypes(rhjmcv1alpha2.SchemeGroupVersion, &rhjmcv1alpha2.FlightRecorder{},
			&rhjmcv1alpha2.FlightRecorderList{})
		s.AddKnownTypes(rhjmcv1alpha2.SchemeGroupVersion, &rhjmcv1alpha2.Recording{},
			&rhjmcv1alpha2.RecordingList{})

		client = fake.NewFakeClientWithScheme(s, objs...)
		server = test.NewServer(client, messages)
		controller = &recording.ReconcileRecording{
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
			test.NewContainerJFR(), test.NewFlightRecorder(), test.NewTargetPod(),
			test.NewContainerJFRService(), test.NewRecording(),
		}
		messages = []test.WsMessage{
			test.NewDumpMessage(),
			test.NewListMessage("RUNNING", 30000),
		}
	})

	AfterEach(func() {
		// Reset test inputs
		objs = nil
		messages = nil
	})

	Describe("reconciling a request", func() {
		Context("with a new recording", func() {
			It("updates status with recording info", func() {
				expectRecordingStatus(controller, client, messages)
			})
			It("adds finalizer to recording", func() {
				req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "my-recording", Namespace: "default"}}
				_, err := controller.Reconcile(req)
				Expect(err).ToNot(HaveOccurred())

				obj := &rhjmcv1alpha2.Recording{}
				err = client.Get(context.Background(), req.NamespacedName, obj)
				Expect(err).ToNot(HaveOccurred())

				finalizers := obj.GetFinalizers()
				Expect(finalizers).To(ContainElement("recording.finalizer.rhjmc.redhat.com"))
			})
			It("should requeue after 10 seconds", func() {
				req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "my-recording", Namespace: "default"}}
				result, err := controller.Reconcile(req)
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(Equal(reconcile.Result{RequeueAfter: 10 * time.Second}))
			})
		})
		Context("with a new recording that fails", func() {
			BeforeEach(func() {
				objs = []runtime.Object{
					test.NewContainerJFR(), test.NewFlightRecorder(), test.NewTargetPod(),
					test.NewContainerJFRService(), test.NewRecording(),
				}
				messages = []test.WsMessage{
					test.FailMessage(test.NewDumpMessage()),
				}
			})
			It("should requeue with error", func() {
				expectReconcileError(controller)
			})
			It("should close Container JFR client", func() {
				req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "my-recording", Namespace: "default"}}
				controller.Reconcile(req)
				Expect(controller.IsClientConnected()).To(BeFalse())
			})
		})
		Context("with a new continuous recording", func() {
			BeforeEach(func() {
				objs = []runtime.Object{
					test.NewContainerJFR(), test.NewFlightRecorder(), test.NewTargetPod(),
					test.NewContainerJFRService(), test.NewContinuousRecording(),
				}
				messages = []test.WsMessage{
					test.NewStartMessage(),
					test.NewListMessage("RUNNING", 0),
				}
			})
			It("updates status with recording info", func() {
				expectRecordingStatus(controller, client, messages)
			})
		})
		Context("with a new continuous recording that fails", func() {
			BeforeEach(func() {
				objs = []runtime.Object{
					test.NewContainerJFR(), test.NewFlightRecorder(), test.NewTargetPod(),
					test.NewContainerJFRService(), test.NewContinuousRecording(),
				}
				messages = []test.WsMessage{
					test.FailMessage(test.NewStartMessage()),
				}
			})
			It("should requeue with error", func() {
				expectReconcileError(controller)
			})
			It("should close Container JFR client", func() {
				req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "my-recording", Namespace: "default"}}
				controller.Reconcile(req)
				Expect(controller.IsClientConnected()).To(BeFalse())
			})
		})
		Context("with a running recording", func() {
			BeforeEach(func() {
				objs = []runtime.Object{
					test.NewContainerJFR(), test.NewFlightRecorder(), test.NewTargetPod(),
					test.NewContainerJFRService(), test.NewRunningRecording(),
				}
				messages = []test.WsMessage{
					test.NewListMessage("RUNNING", 30000),
				}
			})
			It("should not change status", func() {
				expectStatusUnchanged(controller, client)
			})
			It("should requeue after 10 seconds", func() {
				req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "my-recording", Namespace: "default"}}
				result, err := controller.Reconcile(req)
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(Equal(reconcile.Result{RequeueAfter: 10 * time.Second}))
			})
		})
		Context("with a running recording not found in Container JFR", func() {
			BeforeEach(func() {
				objs = []runtime.Object{
					test.NewContainerJFR(), test.NewFlightRecorder(), test.NewTargetPod(),
					test.NewContainerJFRService(), test.NewRunningRecording(),
				}
				messages = []test.WsMessage{
					test.NewListEmptyMessage(),
				}
			})
			It("should not change status", func() {
				expectStatusUnchanged(controller, client)
			})
		})
		Context("when listing recordings fail", func() {
			BeforeEach(func() {
				objs = []runtime.Object{
					test.NewContainerJFR(), test.NewFlightRecorder(), test.NewTargetPod(),
					test.NewContainerJFRService(), test.NewRunningRecording(),
				}
				messages = []test.WsMessage{
					test.FailMessage(test.NewListMessage("RUNNING", 30000)),
				}
			})
			It("should requeue with error", func() {
				expectReconcileError(controller)
			})
			It("should close Container JFR client", func() {
				req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "my-recording", Namespace: "default"}}
				controller.Reconcile(req)
				Expect(controller.IsClientConnected()).To(BeFalse())
			})
		})
		Context("when listing recordings has unexpected state", func() {
			BeforeEach(func() {
				objs = []runtime.Object{
					test.NewContainerJFR(), test.NewFlightRecorder(), test.NewTargetPod(),
					test.NewContainerJFRService(), test.NewRunningRecording(),
				}
				messages = []test.WsMessage{
					test.NewListMessage("DOES-NOT-EXIST", 30000),
				}
			})
			It("should requeue with error", func() {
				expectReconcileError(controller)
			})
		})
		Context("with a running recording to be stopped", func() {
			BeforeEach(func() {
				objs = []runtime.Object{
					test.NewContainerJFR(), test.NewFlightRecorder(), test.NewTargetPod(),
					test.NewContainerJFRService(), test.NewRecordingToStop(),
				}
				messages = []test.WsMessage{
					test.NewStopMessage(),
					test.NewListMessage("STOPPED", 0),
				}
			})
			It("should stop recording", func() {
				expectRecordingStatus(controller, client, messages)
			})
		})
		Context("with a running recording to be stopped that fails", func() {
			BeforeEach(func() {
				objs = []runtime.Object{
					test.NewContainerJFR(), test.NewFlightRecorder(), test.NewTargetPod(),
					test.NewContainerJFRService(), test.NewRecordingToStop(),
				}
				messages = []test.WsMessage{
					test.FailMessage(test.NewStopMessage()),
				}
			})
			It("should requeue with error", func() {
				expectReconcileError(controller)
			})
			It("should close Container JFR client", func() {
				req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "my-recording", Namespace: "default"}}
				controller.Reconcile(req)
				Expect(controller.IsClientConnected()).To(BeFalse())
			})
		})
		Context("with a stopped recording to be archived", func() {
			BeforeEach(func() {
				objs = []runtime.Object{
					test.NewContainerJFR(), test.NewFlightRecorder(), test.NewTargetPod(),
					test.NewContainerJFRService(), test.NewStoppedRecordingToArchive(),
				}
				messages = []test.WsMessage{
					test.NewListMessage("STOPPED", 30000),
					test.NewListSavedEmptyMessage(),
					test.NewSaveMessage(),
					test.NewListSavedMessage(),
				}
			})
			It("should update download URL", func() {
				req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "my-recording", Namespace: "default"}}

				before := &rhjmcv1alpha2.Recording{}
				err := client.Get(context.Background(), req.NamespacedName, before)
				Expect(err).ToNot(HaveOccurred())

				_, err = controller.Reconcile(req)
				Expect(err).ToNot(HaveOccurred())

				after := &rhjmcv1alpha2.Recording{}
				err = client.Get(context.Background(), req.NamespacedName, after)
				Expect(err).ToNot(HaveOccurred())

				// Should all be the same except for Download URL
				Expect(after.Status.State).To(Equal(before.Status.State))
				Expect(after.Status.Duration).To(Equal(before.Status.Duration))
				Expect(after.Status.StartTime).To(Equal(before.Status.StartTime))
				Expect(after.Status.DownloadURL).ToNot(BeNil())
				Expect(*after.Status.DownloadURL).To(Equal("http://path/to/saved-test-recording.jfr"))
			})
			It("should not requeue", func() {
				req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "my-recording", Namespace: "default"}}
				result, err := controller.Reconcile(req)
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(Equal(reconcile.Result{}))
			})
		})
		Context("when listing saved recordings fails", func() {
			BeforeEach(func() {
				objs = []runtime.Object{
					test.NewContainerJFR(), test.NewFlightRecorder(), test.NewTargetPod(),
					test.NewContainerJFRService(), test.NewStoppedRecordingToArchive(),
				}
				messages = []test.WsMessage{
					test.NewListMessage("STOPPED", 30000),
					test.NewListSavedMessage(),
					test.FailMessage(test.NewSaveMessage()),
				}
			})
			It("should requeue with error", func() {
				expectReconcileError(controller)
			})
			It("should close Container JFR client", func() {
				req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "my-recording", Namespace: "default"}}
				controller.Reconcile(req)
				Expect(controller.IsClientConnected()).To(BeFalse())
			})
		})
		Context("when archiving recording fails", func() {
			BeforeEach(func() {
				objs = []runtime.Object{
					test.NewContainerJFR(), test.NewFlightRecorder(), test.NewTargetPod(),
					test.NewContainerJFRService(), test.NewStoppedRecordingToArchive(),
				}
				messages = []test.WsMessage{
					test.NewListMessage("STOPPED", 30000),
					test.FailMessage(test.NewListSavedMessage()),
				}
			})
			It("should requeue with error", func() {
				expectReconcileError(controller)
			})
			It("should close Container JFR client", func() {
				req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "my-recording", Namespace: "default"}}
				controller.Reconcile(req)
				Expect(controller.IsClientConnected()).To(BeFalse())
			})
		})
		Context("with a running recording to be stopped and archived", func() {
			BeforeEach(func() {
				objs = []runtime.Object{
					test.NewContainerJFR(), test.NewFlightRecorder(), test.NewTargetPod(),
					test.NewContainerJFRService(), test.NewRecordingToStopAndArchive(),
				}
				messages = []test.WsMessage{
					test.NewStopMessage(),
					test.NewListMessage("STOPPED", 30000),
					test.NewListSavedEmptyMessage(),
					test.NewSaveMessage(),
					test.NewListSavedMessage(),
				}
			})
			It("should stop recording", func() {
				req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "my-recording", Namespace: "default"}}
				_, err := controller.Reconcile(req)
				Expect(err).ToNot(HaveOccurred())

				obj := &rhjmcv1alpha2.Recording{}
				err = client.Get(context.Background(), req.NamespacedName, obj)
				Expect(err).ToNot(HaveOccurred())

				Expect(obj.Status.State).ToNot(BeNil())
				Expect(*obj.Status.State).To(Equal(rhjmcv1alpha2.RecordingStateStopped))
			})
			It("should update download URL", func() {
				req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "my-recording", Namespace: "default"}}
				_, err := controller.Reconcile(req)
				Expect(err).ToNot(HaveOccurred())

				obj := &rhjmcv1alpha2.Recording{}
				err = client.Get(context.Background(), req.NamespacedName, obj)
				Expect(err).ToNot(HaveOccurred())

				Expect(obj.Status.DownloadURL).ToNot(BeNil())
				Expect(*obj.Status.DownloadURL).To(Equal("http://path/to/saved-test-recording.jfr"))
			})
			It("should not requeue", func() {
				req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "my-recording", Namespace: "default"}}
				result, err := controller.Reconcile(req)
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(Equal(reconcile.Result{}))
			})
		})
		Context("with an archived recording", func() {
			BeforeEach(func() {
				objs = []runtime.Object{
					test.NewContainerJFR(), test.NewFlightRecorder(), test.NewTargetPod(),
					test.NewContainerJFRService(), test.NewArchivedRecording(),
				}
				messages = []test.WsMessage{
					test.NewListMessage("STOPPED", 30000),
					test.NewListSavedMessage(),
				}
			})
			It("should not change status", func() {
				expectStatusUnchanged(controller, client)
			})
		})
		Context("Recording does not exist", func() {
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
					test.NewRecording(),
				}
				messages = []test.WsMessage{}
			})
			It("should requeue", func() {
				req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "my-recording", Namespace: "default"}}
				result, err := controller.Reconcile(req)
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(Equal(reconcile.Result{RequeueAfter: time.Second}))
			})
		})
		Context("Container JFR CR is missing", func() {
			BeforeEach(func() {
				objs = []runtime.Object{
					test.NewFlightRecorder(), test.NewTargetPod(), test.NewContainerJFRService(),
					test.NewRecording(),
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
					test.NewRecording(),
				}
				messages = []test.WsMessage{}
			})
			It("should requeue with error", func() {
				expectReconcileError(controller)
			})
		})
		Context("FlightRecorder is missing", func() {
			BeforeEach(func() {
				objs = []runtime.Object{
					test.NewContainerJFR(), test.NewTargetPod(), test.NewContainerJFRService(),
					test.NewRecording(),
				}
				messages = []test.WsMessage{}
			})
			It("should not requeue", func() {
				req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "my-recording", Namespace: "default"}}
				result, err := controller.Reconcile(req)
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(Equal(reconcile.Result{}))
			})
		})
		Context("Target pod is missing", func() {
			BeforeEach(func() {
				objs = []runtime.Object{
					test.NewContainerJFR(), test.NewFlightRecorder(), test.NewContainerJFRService(),
					test.NewRecording(),
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
					test.NewRecording(),
				}
				messages = []test.WsMessage{}
			})
			It("should requeue with error", func() {
				expectReconcileError(controller)
			})
		})
	})
})

func expectRecordingStatus(controller *recording.ReconcileRecording, client client.Client, messages []test.WsMessage) {
	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "my-recording", Namespace: "default"}}
	_, err := controller.Reconcile(req)
	Expect(err).ToNot(HaveOccurred())

	obj := &rhjmcv1alpha2.Recording{}
	err = client.Get(context.Background(), req.NamespacedName, obj)
	Expect(err).ToNot(HaveOccurred())

	desc := messages[1].Reply.Payload.([]jfrclient.RecordingDescriptor)[0]
	Expect(obj.Status.State).ToNot(BeNil())
	Expect(*obj.Status.State).To(Equal(rhjmcv1alpha2.RecordingState(desc.State)))
	// Converted to RFC3339 during serialization (sub-second precision lost)
	Expect(obj.Status.StartTime).To(Equal(metav1.Unix(0, desc.StartTime*int64(time.Millisecond)).Rfc3339Copy()))
	Expect(obj.Status.Duration).To(Equal(metav1.Duration{
		Duration: time.Duration(desc.Duration) * time.Millisecond,
	}))
	Expect(obj.Status.DownloadURL).ToNot(BeNil())
	Expect(*obj.Status.DownloadURL).To(Equal(desc.DownloadURL))
}

func expectStatusUnchanged(controller *recording.ReconcileRecording, client client.Client) {
	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "my-recording", Namespace: "default"}}

	before := &rhjmcv1alpha2.Recording{}
	err := client.Get(context.Background(), req.NamespacedName, before)
	Expect(err).ToNot(HaveOccurred())

	_, err = controller.Reconcile(req)
	Expect(err).ToNot(HaveOccurred())

	after := &rhjmcv1alpha2.Recording{}
	err = client.Get(context.Background(), req.NamespacedName, after)
	Expect(err).ToNot(HaveOccurred())
	Expect(after.Status).To(Equal(before.Status))
}

func expectReconcileError(controller *recording.ReconcileRecording) {
	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "my-recording", Namespace: "default"}}
	result, err := controller.Reconcile(req)
	Expect(err).To(HaveOccurred())
	Expect(result).To(Equal(reconcile.Result{}))
}
