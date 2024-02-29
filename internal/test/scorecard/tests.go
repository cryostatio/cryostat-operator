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
	"errors"
	"fmt"
	"io"
	"net/url"
	"time"

	scapiv1alpha3 "github.com/operator-framework/api/pkg/apis/scorecard/v1alpha3"
	apimanifests "github.com/operator-framework/api/pkg/manifests"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
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
	_, err = createAndWaitTillCryostatAvailable(newCryostatCR(CryostatCRTestName, namespace, !tr.OpenShift), tr)
	if err != nil {
		return fail(*r, fmt.Sprintf("%s test failed: %s", CryostatCRTestName, err.Error()))
	}
	defer cleanupCryostat(r, tr.Client, CryostatCRTestName, namespace)

	return *r
}

// TODO add a built in discovery test too
func CryostatRecordingTest(bundle *apimanifests.Bundle, namespace string, openShiftCertManager bool) scapiv1alpha3.TestResult {
	tr := newTestResources(CryostatRecordingTestName)
	r := tr.TestResult

	err := setupCRTestResources(tr, openShiftCertManager)
	if err != nil {
		return fail(*r, fmt.Sprintf("failed to set up %s test: %s", CryostatRecordingTestName, err.Error()))
	}

	// Create a default Cryostat CR
	cr, err := createAndWaitTillCryostatAvailable(newCryostatCR(CryostatRecordingTestName, namespace, !tr.OpenShift), tr)
	if err != nil {
		return fail(*r, fmt.Sprintf("failed to determine application URL: %s", err.Error()))
	}
	defer cleanupCryostat(r, tr.Client, CryostatRecordingTestName, namespace)

	base, err := url.Parse(cr.Status.ApplicationURL)
	if err != nil {
		return fail(*r, fmt.Sprintf("application URL is invalid: %s", err.Error()))
	}

	err = waitTillCryostatReady(base, tr)
	if err != nil {
		return fail(*r, fmt.Sprintf("failed to reach the application: %s", err.Error()))
	}

	apiClient := NewCryostatRESTClientset(base)

	// Create a custom target for test
	targetOptions := &Target{
		ConnectUrl: "service:jmx:rmi:///jndi/rmi://localhost:0/jmxrmi",
		Alias:      "customTarget",
	}
	target, err := apiClient.Targets().Create(context.Background(), targetOptions)
	if err != nil {
		return fail(*r, fmt.Sprintf("failed to create a target: %s", err.Error()))
	}
	r.Log += fmt.Sprintf("created a custom target: %+v\n", target)
	connectUrl := target.ConnectUrl

	jmxSecretName := CryostatRecordingTestName + "-jmx-auth"
	secret, err := tr.Client.CoreV1().Secrets(namespace).Get(context.Background(), jmxSecretName, metav1.GetOptions{})
	if err != nil {
		return fail(*r, fmt.Sprintf("failed to get jmx credentials: %s", err.Error()))
	}

	credential := &Credential{
		UserName:        string(secret.Data["CRYOSTAT_RJMX_USER"]),
		Password:        string(secret.Data["CRYOSTAT_RJMX_PASS"]),
		MatchExpression: fmt.Sprintf("target.alias==\"%s\"", target.Alias),
	}

	err = apiClient.CredentialClient.Create(context.Background(), credential)
	if err != nil {
		return fail(*r, fmt.Sprintf("failed to create stored credential: %s", err.Error()))
	}
	r.Log += fmt.Sprintf("created stored credential with match expression: %s\n", credential.MatchExpression)

	// Wait for Cryostat to update the discovery tree
	time.Sleep(2 * time.Second)

	// Create a recording
	options := &RecordingCreateOptions{
		RecordingName: "scorecard_test_rec",
		Events:        "template=ALL",
		Duration:      0, // Continuous
		ToDisk:        true,
		MaxSize:       0,
		MaxAge:        0,
	}
	rec, err := apiClient.Recordings().Create(context.Background(), connectUrl, options)
	if err != nil {
		return fail(*r, fmt.Sprintf("failed to create a recording: %s", err.Error()))
	}
	r.Log += fmt.Sprintf("created a recording: %+v\n", rec)

	// View the current recording list after creating one
	recs, err := apiClient.Recordings().List(context.Background(), connectUrl)
	if err != nil {
		return fail(*r, fmt.Sprintf("failed to list recordings: %s", err.Error()))
	}
	r.Log += fmt.Sprintf("current list of recordings: %+v\n", recs)

	// Allow the recording to run for 10s
	time.Sleep(30 * time.Second)

	// Archive the recording
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	err = wait.PollImmediateUntilWithContext(ctx, time.Second, func(ctx context.Context) (done bool, err error) {
		archiveName, err := apiClient.Recordings().Archive(context.Background(), connectUrl, rec.Name)
		if errors.Is(err, io.EOF) {
			r.Log += fmt.Sprintf("archiving recording resulted in EOF: %s, trying again", err.Error())
			return false, nil
		}
		if err != nil {
			return true, fmt.Errorf("failed to archive the recording: %s", err.Error())
		}
		r.Log += fmt.Sprintf("archived the recording %s at: %s\n", rec.Name, archiveName)
		return true, nil
	})
	if err != nil {
		return fail(*r, err.Error())
	}

	archives, err := apiClient.Recordings().ListArchives(context.Background(), connectUrl)
	if err != nil {
		return fail(*r, fmt.Sprintf("failed to list archives: %s", err.Error()))
	}
	r.Log += fmt.Sprintf("current list of archives: %+v\n", archives)

	report, err := apiClient.Recordings().GenerateReport(context.Background(), connectUrl, rec)
	if err != nil {
		return fail(*r, fmt.Sprintf("failed to generate report for the recording: %s", err.Error()))
	}
	r.Log += fmt.Sprintf("generated report for the recording %s: %+v\n", rec.Name, report)

	// Stop the recording
	err = apiClient.Recordings().Stop(context.Background(), connectUrl, rec.Name)
	if err != nil {
		return fail(*r, fmt.Sprintf("failed to stop the recording %s: %s", rec.Name, err.Error()))
	}
	// Get the recording to verify its state
	rec, err = apiClient.Recordings().Get(context.Background(), connectUrl, rec.Name)
	if err != nil {
		return fail(*r, fmt.Sprintf("failed to get the recordings: %s", err.Error()))
	}
	if rec.State != "STOPPED" {
		return fail(*r, fmt.Sprintf("recording %s failed to stop: %s", rec.Name, err.Error()))
	}
	r.Log += fmt.Sprintf("stopped the recording: %s\n", rec.Name)

	// Delete the recording
	err = apiClient.Recordings().Delete(context.Background(), connectUrl, rec.Name)
	if err != nil {
		return fail(*r, fmt.Sprintf("failed to delete the recording %s: %s", rec.Name, err.Error()))
	}
	r.Log += fmt.Sprintf("deleted the recording: %s\n", rec.Name)

	// View the current recording list after deleting one
	recs, err = apiClient.Recordings().List(context.Background(), connectUrl)
	if err != nil {
		return fail(*r, fmt.Sprintf("failed to list recordings: %s", err.Error()))
	}
	r.Log += fmt.Sprintf("current list of recordings: %+v\n", recs)

	return *r
}
