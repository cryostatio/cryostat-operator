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

// OperatorInstallTest checks that the operator installed correctly
func OperatorInstallTest(bundle *apimanifests.Bundle, namespace string) scapiv1alpha3.TestResult {
	r := newEmptyTestResult(OperatorInstallTestName)

	// Create a new Kubernetes REST client for this test
	client, err := NewClientset()
	if err != nil {
		return fail(*r, fmt.Sprintf("failed to create client: %s", err.Error()))
	}

	// Poll the deployment until it becomes available or we timeout
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	err = waitForDeploymentAvailability(ctx, client, namespace, operatorDeploymentName, r)
	if err != nil {
		return fail(*r, fmt.Sprintf("operator deployment did not become available: %s", err.Error()))
	}

	return *r
}

// CryostatCRTest checks that the operator installs Cryostat in response to a Cryostat CR
func CryostatCRTest(bundle *apimanifests.Bundle, namespace string, openShiftCertManager bool) scapiv1alpha3.TestResult {
	tr := newTestResources(CryostatCRTestName)
	r := tr.TestResult

	err := setupCRTestResources(tr, openShiftCertManager)
	if err != nil {
		return fail(*r, fmt.Sprintf("failed to set up %s test: %s", CryostatCRTestName, err.Error()))
	}

	// Create a default Cryostat CR
	cr := newCryostatCR(namespace, !tr.OpenShift)
	defer cleanupCryostat(r, tr.Client, namespace)

	err = createAndWaitForCryostat(cr, tr)
	if err != nil {
		return fail(*r, fmt.Sprintf("%s test failed: %s", CryostatCRTestName, err.Error()))
	}
	return *r
}

func CryostatRecordingTest(bundle *apimanifests.Bundle, namespace string, openShiftCertManager bool) scapiv1alpha3.TestResult {
	tr := newTestResources(CryostatCRTestName)
	r := tr.TestResult

	err := setupCRTestResources(tr, openShiftCertManager)
	if err != nil {
		return fail(*r, fmt.Sprintf("failed to set up %s test: %s", CryostatRecordingTestName, err.Error()))
	}

	// Create a default Cryostat CR
	cr := newCryostatCR(namespace, !tr.OpenShift)
	defer cleanupCryostat(r, tr.Client, namespace)

	err = createAndWaitForCryostat(cr, tr)
	if err != nil {
		return fail(*r, fmt.Sprintf("failed to determine application URL: %s", err.Error()))
	}

	apiClient, err := NewCryostatRESTClientset(cr.Status.ApplicationURL)
	if err != nil {
		return fail(*r, fmt.Sprintf("failed to get Cryostat API client: %s", err.Error()))
	}

	// Get a target
	targets, err := apiClient.Targets().List()
	if err != nil {
		return fail(*r, fmt.Sprintf("failed to list discovered targets: %s", err.Error()))
	}
	if len(targets) == 0 {
		return fail(*r, "cryostat failed to discover any targets")
	}
	target := targets[0]
	r.Log += fmt.Sprintf("using target: %+v", target)

	// Create a recording
	options := &RecordingCreateOptions{
		RecordingName: "scorecard-test-rec",
		Events:        "template=ALL",
		Duration:      0,
		ToDisk:        true,
		MaxSize:       0,
		MaxAge:        0,
	}
	recording, err := apiClient.Recordings().Create(target, options)
	if err != nil {
		return fail(*r, fmt.Sprintf("failed to create a recording: %s", err.Error()))
	}
	r.Log += fmt.Sprintf("created a recording: %+v", recording)

	// Archive the recording
	archiveName, err := apiClient.Recordings().Archive(target, recording.Name)
	if err != nil {
		return fail(*r, fmt.Sprintf("failed to archive the recording: %s", err.Error()))
	}
	r.Log += fmt.Sprintf("archived the recording %s at: %s", recording.Name, *archiveName)

	// Stop the recording
	err = apiClient.Recordings().Stop(target, recording.Name)
	if err != nil {
		return fail(*r, fmt.Sprintf("failed to stop the recording %s: %s", recording.Name, err.Error()))
	}

	recording, err = apiClient.Recordings().Get(target, recording.Name)
	if err != nil {
		return fail(*r, fmt.Sprintf("failed to get the recordings: %s", err.Error()))
	}
	if recording == nil {
		return fail(*r, fmt.Sprintf("recording %s does not exist: %s", recording.Name, err.Error()))
	}

	if recording.State != "STOPPED" {
		return fail(*r, fmt.Sprintf("recording %s failed to stop: %s", recording.Name, err.Error()))
	}
	r.Log += fmt.Sprintf("stopped the recording: %s", recording.Name)

	// Delete the recording
	err = apiClient.Recordings().Delete(target, recording.Name)
	if err != nil {
		return fail(*r, fmt.Sprintf("failed to delete the recording %s: %s", recording.Name, err.Error()))
	}
	r.Log += fmt.Sprintf("deleted the recording: %s", recording.Name)

	return *r
}
