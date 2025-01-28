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

package console_test

import (
	"context"
	"strconv"

	"github.com/cryostatio/cryostat-operator/internal/console"
	consoletests "github.com/cryostatio/cryostat-operator/internal/console/test"
	"github.com/cryostatio/cryostat-operator/internal/test"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	configv1 "github.com/openshift/api/config/v1"
	consolev1 "github.com/openshift/api/console/v1"
	openshiftoperatorv1 "github.com/openshift/api/operator/v1"

	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

type pluginTestInput struct {
	client ctrlclient.Client
	objs   []ctrlclient.Object
	*consoletests.PluginTestResources
}

var _ = Describe("Plugin", func() {
	var t *pluginTestInput
	var installer *console.PluginInstaller
	count := 0

	namespaceWithSuffix := func(name string) string {
		return name + "-plugin-" + strconv.Itoa(count)
	}

	BeforeEach(func() {
		ns := namespaceWithSuffix("test")
		t = &pluginTestInput{
			PluginTestResources: &consoletests.PluginTestResources{
				TestResources: &test.TestResources{
					Name:             "cryostat",
					Namespace:        ns,
					TargetNamespaces: []string{ns},
					TLS:              true,
				},
			},
		}
		t.objs = []ctrlclient.Object{
			t.NewNamespace(),
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

		installer = &console.PluginInstaller{
			Client:    t.client,
			Namespace: t.Namespace,
			Scheme:    k8sScheme,
			Log:       logger,
		}
	})

	JustAfterEach(func() {
		for _, obj := range append(t.objs, t.NewConsolePlugin()) {
			err := ctrlclient.IgnoreNotFound(t.client.Delete(ctx, obj))
			Expect(err).ToNot(HaveOccurred())
		}
	})

	AfterEach(func() {
		count++
	})

	Context("installing plugin", func() {
		Context("with preconditions met", func() {
			BeforeEach(func() {
				t.objs = append(t.objs, t.NewConsole(), t.NewPluginClusterRoleBinding(), t.NewOperatorDeployment(), t.NewClusterVersion())
			})
			JustBeforeEach(func() {
				t.updateClusterVersionStatus(t.NewClusterVersion())
				err := installer.Start(context.Background())
				Expect(err).ToNot(HaveOccurred())
			})
			It("should create ConsolePlugin", func() {
				expected := t.NewConsolePlugin()
				actual := t.getConsolePlugin(expected)
				Expect(actual.Spec).To(Equal(expected.Spec))
				Expect(actual.OwnerReferences).To(HaveLen(1))
				Expect(actual.OwnerReferences[0].Kind).To(Equal("ClusterRoleBinding"))
				Expect(actual.OwnerReferences[0].Name).To(Equal(t.NewPluginClusterRoleBinding().Name))
				Expect(actual.Labels).To(Equal(expected.Labels))
			})
			It("should update Console", func() {
				expected := t.NewConsoleExisting()
				actual := t.getConsole(expected)
				Expect(actual.Spec.Plugins).To(ConsistOf(expected.Spec.Plugins))
			})
		})
		Context("with plugin already registered", func() {
			BeforeEach(func() {
				t.objs = append(t.objs, t.NewConsoleExisting(), t.NewPluginClusterRoleBinding(), t.NewOperatorDeployment(), t.NewClusterVersion())
			})
			JustBeforeEach(func() {
				t.updateClusterVersionStatus(t.NewClusterVersion())
				err := installer.Start(context.Background())
				Expect(err).ToNot(HaveOccurred())
			})
			It("should do nothing", func() {
				expected := t.NewConsoleExisting()
				actual := t.getConsole(expected)
				Expect(actual.Spec.Plugins).To(ConsistOf(expected.Spec.Plugins))
			})
		})
		Context("with missing owner", func() {
			ExpectUnownedPlugin := func() {
				It("should create ConsolePlugin unowned", func() {
					expected := t.NewConsolePlugin()
					actual := t.getConsolePlugin(expected)
					Expect(actual.Spec).To(Equal(expected.Spec))
					Expect(actual.OwnerReferences).To(BeEmpty())
					Expect(actual.Labels).To(Equal(expected.Labels))
				})
			}
			JustBeforeEach(func() {
				t.updateClusterVersionStatus(t.NewClusterVersion())
				err := installer.Start(context.Background())
				Expect(err).ToNot(HaveOccurred())
			})
			Context("with missing Deployment", func() {
				BeforeEach(func() {
					t.objs = append(t.objs, t.NewConsole(), t.NewPluginClusterRoleBinding(), t.NewClusterVersion())
				})
				ExpectUnownedPlugin()
			})
			Context("with missing Deployment labels", func() {
				BeforeEach(func() {
					t.objs = append(t.objs, t.NewConsole(), t.NewOperatorDeploymentMissingLabels(), t.NewPluginClusterRoleBinding(), t.NewClusterVersion())
				})
				ExpectUnownedPlugin()
			})
			Context("with missing ClusterRoleBinding", func() {
				BeforeEach(func() {
					t.objs = append(t.objs, t.NewConsole(), t.NewOperatorDeployment(), t.NewClusterVersion())
				})
				ExpectUnownedPlugin()
			})
			Context("with missing ClusterRoleBinding labels", func() {
				BeforeEach(func() {
					t.objs = append(t.objs, t.NewConsole(), t.NewOperatorDeployment(), t.NewPluginClusterRoleBindingMissingLabels(), t.NewClusterVersion())
				})
				ExpectUnownedPlugin()
			})
			Context("with missing ClusterRoleBinding service account", func() {
				BeforeEach(func() {
					t.objs = append(t.objs, t.NewConsole(), t.NewOperatorDeployment(), t.NewPluginClusterRoleBindingMissingSA(), t.NewClusterVersion())
				})
				ExpectUnownedPlugin()
			})
		})
		Context("with missing Console", func() {
			BeforeEach(func() {
				t.objs = append(t.objs, t.NewPluginClusterRoleBinding(), t.NewOperatorDeployment(), t.NewClusterVersion())
			})
			It("should fail to update Console", func() {
				err := installer.Start(context.Background())
				Expect(err).To(HaveOccurred())
			})
		})
		Context("with incompatible OpenShift", func() {
			var startError error

			expectNoChanges := func() {
				It("should not create ConsolePlugin", func() {
					plugin := &consolev1.ConsolePlugin{}
					err := t.client.Get(context.Background(), types.NamespacedName{Name: t.NewConsolePlugin().Name}, plugin)
					Expect(kerrors.IsNotFound(err)).To(BeTrue())
				})

				It("should not update Console", func() {
					console := t.getConsole(t.NewConsole())
					Expect(console.Spec.Plugins).ToNot(ContainElement(t.NewConsolePlugin().Name))
				})
			}
			BeforeEach(func() {
				t.objs = append(t.objs, t.NewConsole(), t.NewPluginClusterRoleBinding(), t.NewOperatorDeployment())
			})
			Context("that is too old", func() {
				BeforeEach(func() {
					t.objs = append(t.objs, t.NewClusterVersionOld())
				})
				JustBeforeEach(func() {
					t.updateClusterVersionStatus(t.NewClusterVersionOld())
					startError = installer.Start(context.Background())
				})
				It("should not return an error", func() {
					Expect(startError).ToNot(HaveOccurred())
				})
				expectNoChanges()
			})
			Context("that is too malformed", func() {
				BeforeEach(func() {
					t.objs = append(t.objs, t.NewClusterVersionBad())
				})
				JustBeforeEach(func() {
					t.updateClusterVersionStatus(t.NewClusterVersionBad())
					startError = installer.Start(context.Background())
				})
				It("should return an error", func() {
					Expect(startError).To(HaveOccurred())
				})
				expectNoChanges()
			})
		})
	})
	Context("as runnable", func() {
		It("should need leader election", func() {
			Expect(installer.NeedLeaderElection()).To(BeTrue())
		})
	})

})

func (t *pluginTestInput) getConsolePlugin(expected *consolev1.ConsolePlugin) *consolev1.ConsolePlugin {
	plugin := &consolev1.ConsolePlugin{}
	err := t.client.Get(context.Background(), types.NamespacedName{Name: expected.Name}, plugin)
	Expect(err).ToNot(HaveOccurred())
	return plugin
}

func (t *pluginTestInput) getConsole(expected *openshiftoperatorv1.Console) *openshiftoperatorv1.Console {
	console := &openshiftoperatorv1.Console{}
	err := t.client.Get(context.Background(), types.NamespacedName{Name: expected.Name}, console)
	Expect(err).ToNot(HaveOccurred())
	return console
}

func (t *pluginTestInput) updateClusterVersionStatus(expected *configv1.ClusterVersion) {
	version := &configv1.ClusterVersion{}
	err := t.client.Get(context.Background(), types.NamespacedName{Name: expected.Name}, version)
	Expect(err).ToNot(HaveOccurred())

	version.Status = expected.Status
	err = t.client.Status().Update(context.Background(), version)
	Expect(err).ToNot(HaveOccurred())
}
