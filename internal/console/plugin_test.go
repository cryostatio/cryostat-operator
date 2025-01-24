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
	consolev1 "github.com/openshift/api/console/v1"
	openshiftoperatorv1 "github.com/openshift/api/operator/v1"

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

	Context("installing plugin", func() {
		var installer *console.PluginInstaller

		JustBeforeEach(func() {
			installer = &console.PluginInstaller{
				Client:    t.client,
				Namespace: t.Namespace,
				Scheme:    k8sScheme,
				Log:       logger,
			}
		})
		Context("with preconditions met", func() {
			BeforeEach(func() {
				t.objs = append(t.objs, t.NewConsole(), t.NewPluginClusterRoleBinding())
			})
			JustBeforeEach(func() {
				err := installer.InstallConsolePlugin(context.Background())
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
				t.objs = append(t.objs, t.NewConsoleExisting(), t.NewPluginClusterRoleBinding())
			})
			JustBeforeEach(func() {
				err := installer.InstallConsolePlugin(context.Background())
				Expect(err).ToNot(HaveOccurred())
			})
			It("should do nothing", func() {
				expected := t.NewConsoleExisting()
				actual := t.getConsole(expected)
				Expect(actual.Spec.Plugins).To(ConsistOf(expected.Spec.Plugins))
			})
		})
		Context("with missing ClusterRoleBinding", func() {
			BeforeEach(func() {
				t.objs = append(t.objs, t.NewConsole())
			})
			It("should fail to create ConsolePlugin", func() {
				err := installer.InstallConsolePlugin(context.Background())
				Expect(err).To(HaveOccurred())
			})
		})
		Context("with missing Console", func() {
			BeforeEach(func() {
				t.objs = append(t.objs, t.NewPluginClusterRoleBinding())
			})
			It("should fail to update Console", func() {
				err := installer.InstallConsolePlugin(context.Background())
				Expect(err).To(HaveOccurred())
			})
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
