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
			test.NewContainerJFRService(), test.NewRecording(false),
		}
		messages = []test.WsMessage{
			{
				ExpectedMsg: jfrclient.NewCommandMessage(
					"dump",
					"1.2.3.4:8001",
					types.UID("0"),
					"test-recording",
					"30",
					"jdk.socketRead:enabled=true,jdk.socketWrite:enabled=true"),
				Reply: &jfrclient.ResponseMessage{
					ID:          types.UID("0"),
					CommandName: "dump",
					Status:      jfrclient.ResponseStatusSuccess,
					Payload:     "",
				},
			},
			{
				ExpectedMsg: jfrclient.NewCommandMessage(
					"list",
					"1.2.3.4:8001",
					types.UID("1")),
				Reply: &jfrclient.ResponseMessage{
					ID:          types.UID("1"),
					CommandName: "list",
					Status:      jfrclient.ResponseStatusSuccess,
					Payload: []jfrclient.RecordingDescriptor{
						{
							Name:        "test-recording",
							State:       "RUNNING",
							StartTime:   1597090030341,
							Duration:    30000,
							DownloadURL: "http://path/to/test-recording.jfr",
						},
					},
				},
			},
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
		Context("with a new continuous recording", func() {
			BeforeEach(func() {
				objs = []runtime.Object{
					test.NewContainerJFR(), test.NewFlightRecorder(), test.NewTargetPod(),
					test.NewContainerJFRService(), test.NewRecording(true),
				}
				messages = []test.WsMessage{
					{
						ExpectedMsg: jfrclient.NewCommandMessage(
							"start",
							"1.2.3.4:8001",
							types.UID("0"),
							"test-recording",
							"jdk.socketRead:enabled=true,jdk.socketWrite:enabled=true"),
						Reply: &jfrclient.ResponseMessage{
							ID:          types.UID("0"),
							CommandName: "start",
							Status:      jfrclient.ResponseStatusSuccess,
							Payload:     "http://path/to/test-recording.jfr",
						},
					},
					{
						ExpectedMsg: jfrclient.NewCommandMessage(
							"list",
							"1.2.3.4:8001",
							types.UID("1")),
						Reply: &jfrclient.ResponseMessage{
							ID:          types.UID("1"),
							CommandName: "list",
							Status:      jfrclient.ResponseStatusSuccess,
							Payload: []jfrclient.RecordingDescriptor{
								{
									Name:        "test-recording",
									State:       "RUNNING",
									StartTime:   1597090030341,
									Duration:    0,
									DownloadURL: "http://path/to/test-recording.jfr",
								},
							},
						},
					},
				}
			})
			It("updates status with recording info", func() {
				expectRecordingStatus(controller, client, messages)
			})
		})
		Context("with a running recording", func() {
			BeforeEach(func() {
				objs = []runtime.Object{
					test.NewContainerJFR(), test.NewFlightRecorder(), test.NewTargetPod(),
					test.NewContainerJFRService(), test.NewRunningRecording(false),
				}
				messages = []test.WsMessage{
					{
						ExpectedMsg: jfrclient.NewCommandMessage(
							"list",
							"1.2.3.4:8001",
							types.UID("0")),
						Reply: &jfrclient.ResponseMessage{
							ID:          types.UID("0"),
							CommandName: "list",
							Status:      jfrclient.ResponseStatusSuccess,
							Payload: []jfrclient.RecordingDescriptor{
								{
									Name:        "test-recording",
									State:       "RUNNING",
									StartTime:   1597090030341,
									Duration:    30000,
									DownloadURL: "http://path/to/test-recording.jfr",
								},
							},
						},
					},
				}
			})
			It("should not change status", func() {
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
			})
			It("should requeue after 10 seconds", func() {
				req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "my-recording", Namespace: "default"}}
				result, err := controller.Reconcile(req)
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(Equal(reconcile.Result{RequeueAfter: 10 * time.Second}))
			})
		})
		Context("with a running recording to be stopped", func() {
			BeforeEach(func() {
				state := rhjmcv1alpha2.RecordingStateStopped
				rec := test.NewRunningRecording(true)
				rec.Spec.State = &state
				rec.Spec.Archive = false
				objs = []runtime.Object{
					test.NewContainerJFR(), test.NewFlightRecorder(), test.NewTargetPod(),
					test.NewContainerJFRService(), rec,
				}
				messages = []test.WsMessage{
					{
						ExpectedMsg: jfrclient.NewCommandMessage(
							"stop",
							"1.2.3.4:8001",
							types.UID("0"),
							"test-recording"),
						Reply: &jfrclient.ResponseMessage{
							ID:          types.UID("0"),
							CommandName: "stop",
							Status:      jfrclient.ResponseStatusSuccess,
							Payload:     "",
						},
					},
					{
						ExpectedMsg: jfrclient.NewCommandMessage(
							"list",
							"1.2.3.4:8001",
							types.UID("1")),
						Reply: &jfrclient.ResponseMessage{
							ID:          types.UID("1"),
							CommandName: "list",
							Status:      jfrclient.ResponseStatusSuccess,
							Payload: []jfrclient.RecordingDescriptor{
								{
									Name:        "test-recording",
									State:       "STOPPED",
									StartTime:   1597090030341,
									Duration:    30000,
									DownloadURL: "http://path/to/test-recording.jfr",
								},
							},
						},
					},
				}
			})
			It("should stop recording", func() {
				expectRecordingStatus(controller, client, messages)
			})
		})
		Context("with a stopped recording to be archived", func() {
			BeforeEach(func() {
				state := rhjmcv1alpha2.RecordingStateStopped
				rec := test.NewRunningRecording(true)
				rec.Status.State = &state
				objs = []runtime.Object{
					test.NewContainerJFR(), test.NewFlightRecorder(), test.NewTargetPod(),
					test.NewContainerJFRService(), rec,
				}
				messages = []test.WsMessage{
					{
						ExpectedMsg: jfrclient.NewCommandMessage(
							"list",
							"1.2.3.4:8001",
							types.UID("0")),
						Reply: &jfrclient.ResponseMessage{
							ID:          types.UID("0"),
							CommandName: "list",
							Status:      jfrclient.ResponseStatusSuccess,
							Payload: []jfrclient.RecordingDescriptor{
								{
									Name:        "test-recording",
									State:       "STOPPED",
									StartTime:   1597090030341,
									Duration:    30000,
									DownloadURL: "http://path/to/test-recording.jfr",
								},
							},
						},
					},
					{
						ExpectedMsg: jfrclient.NewControlMessage(
							"list-saved",
							types.UID("1")),
						Reply: &jfrclient.ResponseMessage{
							ID:          types.UID("1"),
							CommandName: "list-saved",
							Status:      jfrclient.ResponseStatusSuccess,
							Payload:     []jfrclient.SavedRecording{},
						},
					},
					{
						ExpectedMsg: jfrclient.NewCommandMessage(
							"save",
							"1.2.3.4:8001",
							types.UID("2"),
							"test-recording"),
						Reply: &jfrclient.ResponseMessage{
							ID:          types.UID("2"),
							CommandName: "save",
							Status:      jfrclient.ResponseStatusSuccess,
							Payload:     "saved-test-recording.jfr",
						},
					},
					{
						ExpectedMsg: jfrclient.NewControlMessage(
							"list-saved",
							types.UID("3")),
						Reply: &jfrclient.ResponseMessage{
							ID:          types.UID("3"),
							CommandName: "list-saved",
							Status:      jfrclient.ResponseStatusSuccess,
							Payload: []jfrclient.SavedRecording{
								{
									Name:        "saved-test-recording.jfr",
									DownloadURL: "http://path/to/saved-test-recording.jfr",
									ReportURL:   "http://path/to/saved-test-recording.html",
								},
							},
						},
					},
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
		Context("with a running recording to be stopped and archived", func() {
			BeforeEach(func() {
				state := rhjmcv1alpha2.RecordingStateStopped
				rec := test.NewRunningRecording(true)
				rec.Spec.State = &state
				objs = []runtime.Object{
					test.NewContainerJFR(), test.NewFlightRecorder(), test.NewTargetPod(),
					test.NewContainerJFRService(), rec,
				}
				messages = []test.WsMessage{
					{
						ExpectedMsg: jfrclient.NewCommandMessage(
							"stop",
							"1.2.3.4:8001",
							types.UID("0"),
							"test-recording"),
						Reply: &jfrclient.ResponseMessage{
							ID:          types.UID("0"),
							CommandName: "stop",
							Status:      jfrclient.ResponseStatusSuccess,
							Payload:     "",
						},
					},
					{
						ExpectedMsg: jfrclient.NewCommandMessage(
							"list",
							"1.2.3.4:8001",
							types.UID("1")),
						Reply: &jfrclient.ResponseMessage{
							ID:          types.UID("1"),
							CommandName: "list",
							Status:      jfrclient.ResponseStatusSuccess,
							Payload: []jfrclient.RecordingDescriptor{
								{
									Name:        "test-recording",
									State:       "STOPPED",
									StartTime:   1597090030341,
									Duration:    30000,
									DownloadURL: "http://path/to/test-recording.jfr",
								},
							},
						},
					},
					{
						ExpectedMsg: jfrclient.NewControlMessage(
							"list-saved",
							types.UID("2")),
						Reply: &jfrclient.ResponseMessage{
							ID:          types.UID("2"),
							CommandName: "list-saved",
							Status:      jfrclient.ResponseStatusSuccess,
							Payload:     []jfrclient.SavedRecording{},
						},
					},
					{
						ExpectedMsg: jfrclient.NewCommandMessage(
							"save",
							"1.2.3.4:8001",
							types.UID("3"),
							"test-recording"),
						Reply: &jfrclient.ResponseMessage{
							ID:          types.UID("3"),
							CommandName: "save",
							Status:      jfrclient.ResponseStatusSuccess,
							Payload:     "saved-test-recording.jfr",
						},
					},
					{
						ExpectedMsg: jfrclient.NewControlMessage(
							"list-saved",
							types.UID("4")),
						Reply: &jfrclient.ResponseMessage{
							ID:          types.UID("4"),
							CommandName: "list-saved",
							Status:      jfrclient.ResponseStatusSuccess,
							Payload: []jfrclient.SavedRecording{
								{
									Name:        "saved-test-recording.jfr",
									DownloadURL: "http://path/to/saved-test-recording.jfr",
									ReportURL:   "http://path/to/saved-test-recording.html",
								},
							},
						},
					},
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

func expectDownloadURLUpdated(controller *recording.ReconcileRecording, client client.Client, newURL string) {

}
