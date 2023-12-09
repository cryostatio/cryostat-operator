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

package scorecard

import (
	"context"
	"fmt"

	scapiv1alpha3 "github.com/operator-framework/api/pkg/apis/scorecard/v1alpha3"
	apimanifests "github.com/operator-framework/api/pkg/manifests"
)

const (
	OperatorInstallTestName   string = "operator-install"
	CryostatCRTestName        string = "cryostat-cr"
	CryostatRecordingTestName string = "cryostat-recording"
)

func commonCRTestSetup(testName string, openShiftCertManager bool) (*CryostatClientset, scapiv1alpha3.TestResult) {
	r := newEmptyTestResult(testName)
	// Create a new Kubernetes REST client for this test
	client, err := NewClientset()
	if err != nil {
		return nil, fail(r, fmt.Sprintf("failed to create client: %s", err.Error()))
	}
	if openShiftCertManager {
		err := installOpenShiftCertManager(&r)
		if err != nil {
			return client, fail(r, fmt.Sprintf("failed to install cert-manager Operator for Red Hat OpenShift: %s", err.Error()))
		}
	}
	return client, r
}

// OperatorInstallTest checks that the operator installed correctly
func OperatorInstallTest(bundle *apimanifests.Bundle, namespace string) scapiv1alpha3.TestResult {
	r := newEmptyTestResult(OperatorInstallTestName)

	// Create a new Kubernetes REST client for this test
	client, err := NewClientset()
	if err != nil {
		return fail(r, fmt.Sprintf("failed to create client: %s", err.Error()))
	}

	// Poll the deployment until it becomes available or we timeout
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	err = waitForDeploymentAvailability(ctx, client, namespace, operatorDeploymentName, &r)
	if err != nil {
		return fail(r, fmt.Sprintf("operator deployment did not become available: %s", err.Error()))
	}

	return r
}

// CryostatCRTest checks that the operator installs Cryostat in response to a Cryostat CR
func CryostatCRTest(bundle *apimanifests.Bundle, namespace string, openShiftCertManager bool) scapiv1alpha3.TestResult {
	client, r := commonCRTestSetup(CryostatCRTestName, openShiftCertManager)
	if r.State != scapiv1alpha3.PassState {
		return r
	}
	openshift, err := isOpenShift(client)
	if err != nil {
		return fail(r, fmt.Sprintf("could not determine whether platform is OpenShift: %s", err.Error()))
	}
	// Create a default Cryostat CR
	r = createAndWaitForCryostat(newCryostatCR(namespace, !openshift), client, r)
	return cleanupCryostat(r, client, namespace)
}

func CryostatRecordingTest(bundle *apimanifests.Bundle, namespace string, openShiftCertManager bool) scapiv1alpha3.TestResult {
	client, r := commonCRTestSetup(CryostatRecordingTestName, openShiftCertManager)
	if r.State != scapiv1alpha3.PassState {
		return r
	}
	openshift, err := isOpenShift(client)
	if err != nil {
		return fail(r, fmt.Sprintf("could not determine whether platform is OpenShift: %s", err.Error()))
	}
	// Create a default Cryostat CR
	r = createAndWaitForCryostat(newCryostatCR(namespace, !openshift), client, r)

	// Create a recording

	return cleanupCryostat(r, client, namespace)
}
