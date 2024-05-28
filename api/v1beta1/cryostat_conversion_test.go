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

package v1beta1_test

import (
	operatorv1beta1 "github.com/cryostatio/cryostat-operator/api/v1beta1"
	operatorv1beta2 "github.com/cryostatio/cryostat-operator/api/v1beta2"
	"github.com/cryostatio/cryostat-operator/internal/controllers/model"
	"github.com/cryostatio/cryostat-operator/internal/test"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type conversionTestInput struct {
	*test.TestResources
}

var _ = Describe("Cryostat", func() {
	var t *conversionTestInput

	BeforeEach(func() {
		t = &conversionTestInput{
			TestResources: &test.TestResources{
				Name:             "cryostat",
				Namespace:        "test",
				TargetNamespaces: []string{"test"},
			},
		}
	})

	DescribeTable("converting from v1beta1 to v1beta2",
		func(oldFunc func(*test.TestResources) *operatorv1beta1.Cryostat, newFunc func(*test.TestResources) *model.CryostatInstance) {
			oldCR := oldFunc(t.TestResources)
			newCRModel := newFunc(t.TestResources)
			Expect(newCRModel.Object).To(BeAssignableToTypeOf(&operatorv1beta2.Cryostat{}))
			newCR := newCRModel.Object.(*operatorv1beta2.Cryostat)
			// Fill in Status.TargetNamespaces
			newCR.Status.TargetNamespaces = newCR.Spec.TargetNamespaces

			converted := &operatorv1beta2.Cryostat{}
			err := oldCR.ConvertTo(converted)
			Expect(err).ToNot(HaveOccurred())

			Expect(converted).To(Equal(newCR))
		},
		tableEntriesTo(),
	)

	DescribeTable("converting from v1beta2 to v1beta1",
		func(oldFunc func(*test.TestResources) *operatorv1beta1.Cryostat, newFunc func(*test.TestResources) *model.CryostatInstance) {
			oldCR := oldFunc(t.TestResources)
			newCRModel := newFunc(t.TestResources)
			Expect(newCRModel.Object).To(BeAssignableToTypeOf(&operatorv1beta2.Cryostat{}))
			newCR := newCRModel.Object.(*operatorv1beta2.Cryostat)

			converted := &operatorv1beta1.Cryostat{}
			err := converted.ConvertFrom(newCR)
			Expect(err).ToNot(HaveOccurred())

			Expect(converted).To(Equal(oldCR))
		},
		tableEntriesFrom(),
	)

})

func tableEntriesTo() []TableEntry {
	return append(tableEntries(),
		Entry("WS connections", (*test.TestResources).NewCryostatWithWsConnectionsSpecV1Beta1,
			(*test.TestResources).NewCryostat),
		Entry("command config", (*test.TestResources).NewCryostatWithCommandConfigV1Beta1,
			(*test.TestResources).NewCryostatWithIngress),
		Entry("minimal mode", (*test.TestResources).NewCryostatWithMinimalModeV1Beta1,
			(*test.TestResources).NewCryostat),
		Entry("core JMX port", (*test.TestResources).NewCryostatWithCoreSvcJMXPortV1Beta1,
			(*test.TestResources).NewCryostatWithCoreSvc),
	)
}

func tableEntriesFrom() []TableEntry {
	return tableEntries()
}

func tableEntries() []TableEntry {
	return []TableEntry{
		Entry("defaults", (*test.TestResources).NewCryostatV1Beta1,
			(*test.TestResources).NewCryostat),
		Entry("secrets", (*test.TestResources).NewCryostatWithSecretsV1Beta1,
			(*test.TestResources).NewCryostatWithSecrets),
		Entry("templates", (*test.TestResources).NewCryostatWithTemplatesV1Beta1,
			(*test.TestResources).NewCryostatWithTemplates),
		Entry("ingress", (*test.TestResources).NewCryostatWithIngressV1Beta1,
			(*test.TestResources).NewCryostatWithIngress),
		Entry("ingress with cert-manager disabled", (*test.TestResources).NewCryostatWithIngressCertManagerDisabledV1Beta1,
			(*test.TestResources).NewCryostatWithIngressCertManagerDisabled),
		Entry("PVC spec", (*test.TestResources).NewCryostatWithPVCSpecV1Beta1,
			(*test.TestResources).NewCryostatWithPVCSpec),
		Entry("PVC spec with some defaults", (*test.TestResources).NewCryostatWithPVCSpecSomeDefaultV1Beta1,
			(*test.TestResources).NewCryostatWithPVCSpecSomeDefault),
		Entry("PVC spec with labels only", (*test.TestResources).NewCryostatWithPVCLabelsOnlyV1Beta1,
			(*test.TestResources).NewCryostatWithPVCLabelsOnly),
		Entry("default emptyDir", (*test.TestResources).NewCryostatWithDefaultEmptyDirV1Beta1,
			(*test.TestResources).NewCryostatWithDefaultEmptyDir),
		Entry("custom emptyDir", (*test.TestResources).NewCryostatWithEmptyDirSpecV1Beta1,
			(*test.TestResources).NewCryostatWithEmptyDirSpec),
		Entry("core service", (*test.TestResources).NewCryostatWithCoreSvcV1Beta1,
			(*test.TestResources).NewCryostatWithCoreSvc),
		Entry("reports service", (*test.TestResources).NewCryostatWithReportsSvcV1Beta1,
			(*test.TestResources).NewCryostatWithReportsSvc),
		Entry("core ingress", (*test.TestResources).NewCryostatWithCoreNetworkOptionsV1Beta1,
			(*test.TestResources).NewCryostatWithCoreNetworkOptions),
		Entry("reports resources", (*test.TestResources).NewCryostatWithReportsResourcesV1Beta1,
			(*test.TestResources).NewCryostatWithReportsResources),
		Entry("reports low resource limit", (*test.TestResources).NewCryostatWithReportLowResourceLimitV1Beta1,
			(*test.TestResources).NewCryostatWithReportLowResourceLimit),
		Entry("scheduling", (*test.TestResources).NewCryostatWithSchedulingV1Beta1,
			(*test.TestResources).NewCryostatWithScheduling),
		Entry("reports scheduling", (*test.TestResources).NewCryostatWithReportsSchedulingV1Beta1,
			(*test.TestResources).NewCryostatWithReportsScheduling),
		Entry("cert-manager disabled", (*test.TestResources).NewCryostatCertManagerDisabledV1Beta1,
			(*test.TestResources).NewCryostatCertManagerDisabled),
		Entry("cert-manager undefined", (*test.TestResources).NewCryostatCertManagerUndefinedV1Beta1,
			(*test.TestResources).NewCryostatCertManagerUndefined),
		Entry("resources", (*test.TestResources).NewCryostatWithResourcesV1Beta1,
			(*test.TestResources).NewCryostatWithResources),
		Entry("low resource limit", (*test.TestResources).NewCryostatWithLowResourceLimitV1Beta1,
			(*test.TestResources).NewCryostatWithLowResourceLimit),
		Entry("built-in discovery disabled", (*test.TestResources).NewCryostatWithBuiltInDiscoveryDisabledV1Beta1,
			(*test.TestResources).NewCryostatWithBuiltInDiscoveryDisabled),
		Entry("discovery port custom config", (*test.TestResources).NewCryostatWithDiscoveryPortConfigV1Beta1,
			(*test.TestResources).NewCryostatWithDiscoveryPortConfig),
		Entry("discovery port config disabled", (*test.TestResources).NewCryostatWithBuiltInPortConfigDisabledV1Beta1,
			(*test.TestResources).NewCryostatWithBuiltInPortConfigDisabled),
		Entry("JMX cache options", (*test.TestResources).NewCryostatWithJmxCacheOptionsSpecV1Beta1,
			(*test.TestResources).NewCryostatWithJmxCacheOptionsSpec),
		Entry("security", (*test.TestResources).NewCryostatWithSecurityOptionsV1Beta1,
			(*test.TestResources).NewCryostatWithSecurityOptions),
		Entry("reports security", (*test.TestResources).NewCryostatWithReportSecurityOptionsV1Beta1,
			(*test.TestResources).NewCryostatWithReportSecurityOptions),
		Entry("database secret", (*test.TestResources).NewCryostatWithDatabaseSecretProvidedV1Beta1,
			(*test.TestResources).NewCryostatWithDatabaseSecretProvided),
		Entry("metadata", (*test.TestResources).NewCryostatWithAdditionalMetadataV1Beta1,
			(*test.TestResources).NewCryostatWithAdditionalMetadata),
	}
}
