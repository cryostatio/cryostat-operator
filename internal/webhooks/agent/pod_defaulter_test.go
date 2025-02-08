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

package agent_test

import (
	"context"
	"strconv"
	"strings"

	operatorv1beta2 "github.com/cryostatio/cryostat-operator/api/v1beta2"
	"github.com/cryostatio/cryostat-operator/internal/controllers/common"
	"github.com/cryostatio/cryostat-operator/internal/controllers/model"
	"github.com/cryostatio/cryostat-operator/internal/test"
	webhooktests "github.com/cryostatio/cryostat-operator/internal/webhooks/agent/test"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

type defaulterTestInput struct {
	client ctrlclient.Client
	objs   []ctrlclient.Object
	*webhooktests.AgentWebhookTestResources
}

var _ = Describe("PodDefaulter", func() {
	var t *defaulterTestInput
	var otherNS string
	count := 0

	namespaceWithSuffix := func(name string) string {
		return name + "-agent-" + strconv.Itoa(count)
	}

	BeforeEach(func() {
		ns := namespaceWithSuffix("test")
		otherNS = namespaceWithSuffix("other")
		t = &defaulterTestInput{
			AgentWebhookTestResources: &webhooktests.AgentWebhookTestResources{
				TestResources: &test.TestResources{
					Name:             "cryostat",
					Namespace:        ns,
					TargetNamespaces: []string{ns},
					TLS:              true,
				},
			},
		}
		t.objs = []ctrlclient.Object{
			t.NewNamespace(), t.NewOtherNamespace(otherNS),
		}
	})

	JustBeforeEach(func() {
		logger := zap.New()
		logf.SetLogger(logger)

		t.client = k8sClient
		for _, obj := range t.objs {
			err := t.client.Create(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
		}
	})

	JustAfterEach(func() {
		for _, obj := range t.objs {
			err := ctrlclient.IgnoreNotFound(t.client.Delete(ctx, obj))
			Expect(err).ToNot(HaveOccurred())
		}
	})

	AfterEach(func() {
		count++
	})

	Context("configuring a pod", func() {
		var originalPod *corev1.Pod
		var expectedPod *corev1.Pod

		ExpectPod := func() {
			It("should add init container", func() {
				actual := t.getPod(expectedPod)
				expectedInitContainers := expectedPod.Spec.InitContainers
				Expect(actual.Spec.InitContainers).To(HaveLen(len(expectedInitContainers)))
				for idx := range expectedInitContainers {
					expected := expectedPod.Spec.InitContainers[idx]
					container := actual.Spec.InitContainers[idx]
					Expect(container.Name).To(Equal(expected.Name))
					Expect(container.Command).To(Equal(expected.Command))
					Expect(container.Args).To(Equal(expected.Args))
					Expect(container.Env).To(Equal(expected.Env))
					Expect(container.EnvFrom).To(Equal(expected.EnvFrom))
					Expect(container.Image).To(HavePrefix(expected.Image[:strings.Index(expected.Image, ":")]))
					Expect(container.ImagePullPolicy).To(Equal(expected.ImagePullPolicy))
					Expect(container.VolumeMounts).To(Equal(expected.VolumeMounts))
					Expect(container.SecurityContext).To(Equal(expected.SecurityContext))
					Expect(container.Ports).To(Equal(expected.Ports))
					Expect(container.LivenessProbe).To(Equal(expected.LivenessProbe))
					Expect(container.ReadinessProbe).To(Equal(expected.ReadinessProbe))
					test.ExpectResourceRequirements(&container.Resources, &expected.Resources)
				}
			})

			It("should add volume(s)", func() {
				actual := t.getPod(expectedPod)
				Expect(actual.Spec.Volumes).To(ConsistOf(expectedPod.Spec.Volumes))
			})

			It("should add volume mounts(s)", func() {
				actual := t.getPod(expectedPod)
				Expect(actual.Spec.Containers).To(HaveLen(len(expectedPod.Spec.Containers)))
				for i, expected := range expectedPod.Spec.Containers {
					container := actual.Spec.Containers[i]
					Expect(container.VolumeMounts).To(ConsistOf(expected.VolumeMounts))
				}
			})

			It("should add environment variables", func() {
				actual := t.getPod(expectedPod)
				Expect(actual.Spec.Containers).To(HaveLen(len(expectedPod.Spec.Containers)))
				for i, expected := range expectedPod.Spec.Containers {
					container := actual.Spec.Containers[i]
					Expect(container.Env).To(ConsistOf(expected.Env))
					Expect(container.EnvFrom).To(ConsistOf(expected.EnvFrom))
				}
			})

			It("should add ports(s)", func() {
				actual := t.getPod(expectedPod)
				Expect(actual.Spec.Containers).To(HaveLen(len(expectedPod.Spec.Containers)))
				for i, expected := range expectedPod.Spec.Containers {
					container := actual.Spec.Containers[i]
					Expect(container.Ports).To(ConsistOf(expected.Ports))
				}
			})
		}

		Context("with a Cryostat CR", func() {
			JustBeforeEach(func() {
				cr := t.getCryostatInstance()
				cr.Status.TargetNamespaces = cr.Spec.TargetNamespaces
				t.updateCryostatInstanceStatus(cr)

				err := t.client.Create(ctx, originalPod)
				Expect(err).ToNot(HaveOccurred())
			})

			Context("with TLS enabled", func() {
				BeforeEach(func() {
					t.objs = append(t.objs, t.NewCryostat().Object)
					originalPod = t.NewPod()
					expectedPod = t.NewMutatedPod()
				})

				ExpectPod()
			})

			Context("with TLS disabled", func() {
				BeforeEach(func() {
					t.TLS = false
					t.objs = append(t.objs, t.NewCryostatCertManagerDisabled().Object)
					originalPod = t.NewPod()
					expectedPod = t.NewMutatedPod()
				})

				ExpectPod()
			})

			Context("with existing JAVA_TOOL_OPTIONS", func() {
				BeforeEach(func() {
					t.objs = append(t.objs, t.NewCryostat().Object)
					originalPod = t.NewPodJavaToolOptions()
					expectedPod = t.NewMutatedPodJavaToolOptions()
				})

				ExpectPod()
			})

			Context("with existing JAVA_TOOL_OPTIONS using valueFrom", func() {
				BeforeEach(func() {
					t.objs = append(t.objs, t.NewCryostat().Object)
					originalPod = t.NewPodJavaToolOptionsFrom()
					// Should fail
					expectedPod = originalPod
				})

				ExpectPod()
			})

			Context("in a different namespace", func() {
				BeforeEach(func() {
					t.TargetNamespaces = append(t.TargetNamespaces, otherNS)
					t.objs = append(t.objs, t.NewCryostat().Object)
					originalPod = t.NewPodOtherNamespace(otherNS)
					expectedPod = t.NewMutatedPodOtherNamespace(otherNS)
				})

				ExpectPod()
			})

			Context("in a non-target namespace", func() {
				BeforeEach(func() {
					t.objs = append(t.objs, t.NewCryostat().Object)
					originalPod = t.NewPodOtherNamespace(otherNS)
					// Should fail
					expectedPod = originalPod
				})

				ExpectPod()
			})

			Context("with no name label", func() {
				BeforeEach(func() {
					t.objs = append(t.objs, t.NewCryostat().Object)
					originalPod = t.NewPodNoNameLabel()
					// Should fail
					expectedPod = originalPod
				})

				ExpectPod()
			})

			Context("with no namespace label", func() {
				BeforeEach(func() {
					t.objs = append(t.objs, t.NewCryostat().Object)
					originalPod = t.NewPodNoNamespaceLabel()
					// Should fail
					expectedPod = originalPod
				})

				ExpectPod()
			})

			Context("with custom image tag", func() {
				var saveOSUtils common.OSUtils

				BeforeEach(func() {
					t.objs = append(t.objs, t.NewCryostat().Object)
					originalPod = t.NewPod()
				})

				setImageTag := func(imageTag string) {
					saveOSUtils = agentWebhookConfig.OSUtils
					// Force webhook to query environment again
					agentWebhookConfig.InitImageTag = nil
					agentWebhookConfig.OSUtils = test.NewTestOSUtils(&test.TestReconcilerConfig{
						EnvAgentInitImageTag: &[]string{imageTag}[0],
					})
				}

				JustAfterEach(func() {
					// Reset state
					agentWebhookConfig.OSUtils = saveOSUtils
					agentWebhookConfig.InitImageTag = nil
				})

				Context("for development", func() {
					BeforeEach(func() {
						expectedPod = t.NewMutatedPodCustomDevImage()
						setImageTag("example.com/agent-init:latest")
					})

					ExpectPod()
				})

				Context("for release", func() {
					BeforeEach(func() {
						expectedPod = t.NewMutatedPodCustomImage()
						setImageTag("example.com/agent-init:2.0.0")
					})

					ExpectPod()
				})
			})

			Context("with a custom gateway port", func() {
				BeforeEach(func() {
					t.objs = append(t.objs, t.NewCryostatWithAgentGatewaySvc().Object)
					originalPod = t.NewPod()
					expectedPod = t.NewMutatedPodGatewayPort()
				})

				ExpectPod()
			})

			Context("with a custom callback port label", func() {
				Context("that is valid", func() {
					BeforeEach(func() {
						t.objs = append(t.objs, t.NewCryostat().Object)
						originalPod = t.NewPodPortLabel()
						expectedPod = t.NewMutatedPodCallbackPort()
					})

					ExpectPod()
				})

				Context("that is non-integer", func() {
					BeforeEach(func() {
						t.objs = append(t.objs, t.NewCryostat().Object)
						originalPod = t.NewPodPortLabelInvalid()
						// Should fail
						expectedPod = originalPod
					})

					ExpectPod()
				})

				Context("that is too large", func() {
					BeforeEach(func() {
						t.objs = append(t.objs, t.NewCryostat().Object)
						originalPod = t.NewPodPortLabelTooBig()
						// Should fail
						expectedPod = originalPod
					})

					ExpectPod()
				})

				Context("with hostname verification disabled", func() {
					BeforeEach(func() {
						t.DisableAgentHostnameVerify = true
						t.objs = append(t.objs, t.NewCryostatWithAgentHostnameVerifyDisabled().Object)
						originalPod = t.NewPodPortLabel()
						expectedPod = t.NewMutatedPodCallbackPort()
					})

					ExpectPod()
				})
			})

			Context("with hostname verification disabled", func() {
				BeforeEach(func() {
					t.DisableAgentHostnameVerify = true
					t.objs = append(t.objs, t.NewCryostatWithAgentHostnameVerifyDisabled().Object)
					originalPod = t.NewPod()
					expectedPod = t.NewMutatedPod()
				})

				ExpectPod()
			})

			Context("with multiple containers", func() {
				BeforeEach(func() {
					t.objs = append(t.objs, t.NewCryostat().Object)
					originalPod = t.NewPodMultiContainer()
					expectedPod = t.NewMutatedPodMultiContainer()
				})

				ExpectPod()
			})

			Context("with a custom container label", func() {
				Context("for a container that exists", func() {
					BeforeEach(func() {
						t.objs = append(t.objs, t.NewCryostat().Object)
						originalPod = t.NewPodContainerLabel()
						expectedPod = t.NewMutatedPodContainerLabel()
					})

					ExpectPod()
				})

				Context("for a container that doesn't exist", func() {
					BeforeEach(func() {
						t.objs = append(t.objs, t.NewCryostat().Object)
						originalPod = t.NewPodContainerBadLabel()
						// Should fail
						expectedPod = originalPod
					})

					ExpectPod()
				})
			})

			Context("with a custom read-only label", func() {
				Context("that is valid", func() {
					BeforeEach(func() {
						t.objs = append(t.objs, t.NewCryostat().Object)
						originalPod = t.NewPodReadOnlyLabel()
						expectedPod = t.NewMutatedPodReadOnlyLabel()
					})

					ExpectPod()
				})

				Context("that is non-boolean", func() {
					BeforeEach(func() {
						t.objs = append(t.objs, t.NewCryostat().Object)
						originalPod = t.NewPodReadOnlyLabelInvalid()
						// Should fail
						expectedPod = originalPod
					})

					ExpectPod()
				})
			})

			Context("with a custom java options var label", func() {
				BeforeEach(func() {
					t.objs = append(t.objs, t.NewCryostat().Object)
					originalPod = t.NewPodJavaOptsVar()
					expectedPod = t.NewMutatedPodJavaOptsVarLabel()
				})

				ExpectPod()
			})

			Context("with harvester enabled", func() {
				Context("with default exit settings", func() {
					BeforeEach(func() {
						t.objs = append(t.objs, t.NewCryostat().Object)
						originalPod = t.NewPodHarvesterTemplate()
						expectedPod = t.NewMutatedPodHarvesterTemplate()
					})

					ExpectPod()
				})

				Context("with exit age setting", func() {
					Context("that is valid", func() {
						BeforeEach(func() {
							t.objs = append(t.objs, t.NewCryostat().Object)
							originalPod = t.NewPodHarvesterTemplateAge()
							expectedPod = t.NewMutatedPodHarvesterTemplateAge()
						})

						ExpectPod()
					})

					Context("that is invalid", func() {
						BeforeEach(func() {
							t.objs = append(t.objs, t.NewCryostat().Object)
							originalPod = t.NewPodHarvesterTemplateInvalidAge()
							// Should fail
							expectedPod = originalPod
						})

						ExpectPod()
					})
				})

				Context("with exit size setting", func() {
					Context("that is valid", func() {
						BeforeEach(func() {
							t.objs = append(t.objs, t.NewCryostat().Object)
							originalPod = t.NewPodHarvesterTemplateSize()
							expectedPod = t.NewMutatedPodHarvesterTemplateSize()
						})

						ExpectPod()
					})

					Context("that is invalid", func() {
						BeforeEach(func() {
							t.objs = append(t.objs, t.NewCryostat().Object)
							originalPod = t.NewPodHarvesterTemplateInvalidSize()
							// Should fail
							expectedPod = originalPod
						})

						ExpectPod()
					})
				})
			})

			Context("with a custom resource requirements", func() {
				Context("that are valid", func() {
					BeforeEach(func() {
						t.objs = append(t.objs, t.NewCryostatWithAgentInitResources().Object)
						originalPod = t.NewPod()
						expectedPod = t.NewMutatedPodResources()
					})

					ExpectPod()
				})

				Context("with a low limit", func() {
					BeforeEach(func() {
						t.objs = append(t.objs, t.NewCryostatWithAgentInitLowResourceLimit().Object)
						originalPod = t.NewPod()
						expectedPod = t.NewMutatedPodResourcesLowLimit()
					})

					ExpectPod()
				})
			})
		})

		Context("with a missing Cryostat CR", func() {
			BeforeEach(func() {
				originalPod = t.NewPod()
				// Should fail
				expectedPod = originalPod
			})

			JustBeforeEach(func() {
				err := t.client.Create(ctx, originalPod)
				Expect(err).ToNot(HaveOccurred())
			})

			ExpectPod()
		})
	})

})

func (t *defaulterTestInput) getPod(expected *corev1.Pod) *corev1.Pod {
	pod := &corev1.Pod{}
	err := t.client.Get(context.Background(), types.NamespacedName{Name: expected.Name, Namespace: expected.Namespace}, pod)
	Expect(err).ToNot(HaveOccurred())
	return pod
}

func (t *defaulterTestInput) getCryostatInstance() *model.CryostatInstance {
	cr := &operatorv1beta2.Cryostat{}
	err := t.client.Get(context.Background(), types.NamespacedName{Name: t.Name, Namespace: t.Namespace}, cr)
	Expect(err).ToNot(HaveOccurred())
	return t.ConvertNamespacedToModel(cr)
}

func (t *defaulterTestInput) updateCryostatInstanceStatus(cr *model.CryostatInstance) {
	err := t.client.Status().Update(context.Background(), cr.Object)
	Expect(err).ToNot(HaveOccurred())
}
