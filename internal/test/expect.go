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

package test

import (
	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
)

func ExpectResourceRequirements(containerResource, expectedResource *corev1.ResourceRequirements) {
	// Containers must have resource requests
	gomega.Expect(containerResource.Requests).ToNot(gomega.BeNil())

	requestCpu, requestCpuFound := containerResource.Requests[corev1.ResourceCPU]
	expectedRequestCpu := expectedResource.Requests[corev1.ResourceCPU]
	gomega.Expect(requestCpuFound).To(gomega.BeTrue())
	gomega.Expect(requestCpu.Equal(expectedRequestCpu)).To(gomega.BeTrue(),
		"expected CPU requests %s to equal %s", requestCpu.String(), expectedRequestCpu.String())

	requestMemory, requestMemoryFound := containerResource.Requests[corev1.ResourceMemory]
	expectedRequestMemory := expectedResource.Requests[corev1.ResourceMemory]
	gomega.Expect(requestMemoryFound).To(gomega.BeTrue())
	gomega.Expect(requestMemory.Equal(expectedRequestMemory)).To(gomega.BeTrue(),
		"expected memory requests %s to equal %s", requestMemory.String(), expectedRequestMemory.String())

	if expectedResource.Limits == nil {
		gomega.Expect(containerResource.Limits).To(gomega.BeNil())
	} else {
		gomega.Expect(containerResource.Limits).ToNot(gomega.BeNil())

		limitCpu, limitCpuFound := containerResource.Limits[corev1.ResourceCPU]
		expectedLimitCpu, expectedLimitCpuFound := expectedResource.Limits[corev1.ResourceCPU]

		gomega.Expect(limitCpuFound).To(gomega.Equal(expectedLimitCpuFound))
		if expectedLimitCpuFound {
			gomega.Expect(limitCpu.Equal(expectedLimitCpu)).To(gomega.BeTrue(),
				"expected CPU limit %s to equal %s", limitCpu.String(), expectedLimitCpu.String())
		}

		limitMemory, limitMemoryFound := containerResource.Limits[corev1.ResourceMemory]
		expectedlimitMemory, expectedLimitMemoryFound := expectedResource.Limits[corev1.ResourceMemory]

		gomega.Expect(limitMemoryFound).To(gomega.Equal(expectedLimitMemoryFound))
		if expectedLimitCpuFound {
			gomega.Expect(limitMemory.Equal(expectedlimitMemory)).To(gomega.BeTrue(),
				"expected memory limit %s to equal %s", limitMemory.String(), expectedlimitMemory.String())
		}
	}
}
