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
	"net/url"
	"time"

	operatorv1beta2 "github.com/cryostatio/cryostat-operator/api/v1beta2"
	scapiv1alpha3 "github.com/operator-framework/api/pkg/apis/scorecard/v1alpha3"
	apimanifests "github.com/operator-framework/api/pkg/manifests"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	OperatorInstallTestName        string = "operator-install"
	CryostatCRTestName             string = "cryostat-cr"
	CryostatMultiNamespaceTestName string = "cryostat-multi-namespace"
	CryostatRecordingTestName      string = "cryostat-recording"
	CryostatConfigChangeTestName   string = "cryostat-config-change"
	CryostatReportTestName         string = "cryostat-report"
)

// OperatorInstallTest checks that the operator installed correctly
func OperatorInstallTest(bundle *apimanifests.Bundle, namespace string, openShiftCertManager bool) *scapiv1alpha3.TestResult {
	r := newTestResources(OperatorInstallTestName, namespace)

	// Create a new Kubernetes REST client for this test
	err := r.setupCRTestResources(openShiftCertManager)
	if err != nil {
		return r.fail(fmt.Sprintf("failed to set up %s test: %s", OperatorInstallTestName, err.Error()))
	}
	defer r.cleanupAndLogs()

	// Poll the deployment until it becomes available or we timeout
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	err = r.waitForDeploymentAvailability(ctx, operatorDeploymentName, namespace)
	if err != nil {
		return r.fail(fmt.Sprintf("operator deployment did not become available: %s", err.Error()))
	}
	return r.TestResult
}

// CryostatCRTest checks that the operator installs Cryostat in response to a Cryostat CR
func CryostatCRTest(bundle *apimanifests.Bundle, namespace string, openShiftCertManager bool) *scapiv1alpha3.TestResult {
	r := newTestResources(CryostatCRTestName, namespace)

	err := r.setupCRTestResources(openShiftCertManager)
	if err != nil {
		return r.fail(fmt.Sprintf("failed to set up %s test: %s", CryostatCRTestName, err.Error()))
	}
	defer r.cleanupAndLogs()

	// Create a default Cryostat CR
	_, err = r.createAndWaitTillCryostatAvailable(r.newCryostatCR())
	if err != nil {
		return r.fail(fmt.Sprintf("%s test failed: %s", CryostatCRTestName, err.Error()))
	}
	return r.TestResult
}

// CryostatMultiNamespaceTest checks that the operator installs multi-namespace Cryostat in response to a multi-namespace Cryostat CR
func CryostatMultiNamespaceTest(bundle *apimanifests.Bundle, namespace string, openShiftCertManager bool) *scapiv1alpha3.TestResult {
	r := newTestResources(CryostatMultiNamespaceTestName, namespace)
	r.TargetNamespaces = []string{namespace + "-other"}

	err := r.setupCRTestResources(openShiftCertManager)
	if err != nil {
		return r.fail(fmt.Sprintf("failed to set up %s test: %s", CryostatMultiNamespaceTestName, err.Error()))
	}
	defer r.cleanupAndLogs()

	err = r.setupTargetNamespace()
	if err != nil {
		return r.fail(fmt.Sprintf("failed to create target namespaces for %s test: %s", CryostatMultiNamespaceTestName, err.Error()))
	}

	// Create a default ClusterCryostat CR
	_, err = r.createAndWaitTillCryostatAvailable(r.newMultiNamespaceCryostatCR())
	if err != nil {
		return r.fail(fmt.Sprintf("%s test failed: %s", CryostatMultiNamespaceTestName, err.Error()))
	}

	return r.TestResult
}

// CryostatConfigChangeTest checks that the operator redeploys Cryostat in response to a change to Cryostat CR
func CryostatConfigChangeTest(bundle *apimanifests.Bundle, namespace string, openShiftCertManager bool) *scapiv1alpha3.TestResult {
	r := newTestResources(CryostatConfigChangeTestName, namespace)

	err := r.setupCRTestResources(openShiftCertManager)
	if err != nil {
		return r.fail(fmt.Sprintf("failed to set up %s test: %s", CryostatConfigChangeTestName, err.Error()))
	}
	defer r.cleanupAndLogs()

	// Create a default Cryostat CR with default empty dir
	cr := r.newCryostatCR()
	cr.Spec.StorageOptions = &operatorv1beta2.StorageConfiguration{
		EmptyDir: &operatorv1beta2.EmptyDirConfig{
			Enabled: true,
		},
	}

	_, err = r.createAndWaitTillCryostatAvailable(cr)
	if err != nil {
		return r.fail(fmt.Sprintf("failed to determine application URL: %s", err.Error()))
	}

	// Switch Cryostat CR to PVC for redeployment
	cr, err = r.updateAndWaitTillCryostatAvailable(cr)
	if err != nil {
		return r.fail(fmt.Sprintf("Cryostat redeployment did not become available: %s", err.Error()))
	}
	r.Log += "Cryostat deployment has successfully updated with new spec template\n"

	base, err := url.Parse(cr.Status.ApplicationURL)
	r.Log += fmt.Sprintf("base url: %s\n", base)
	if err != nil {
		return r.fail(fmt.Sprintf("application URL is invalid: %s", err.Error()))
	}

	err = r.waitTillCryostatReady(base)
	if err != nil {
		return r.fail(fmt.Sprintf("failed to reach the application: %s", err.Error()))
	}

	return r.TestResult
}

// TODO add a built in discovery test too
func CryostatRecordingTest(bundle *apimanifests.Bundle, namespace string, openShiftCertManager bool) *scapiv1alpha3.TestResult {
	r := newTestResources(CryostatRecordingTestName, namespace)

	err := r.setupCRTestResources(openShiftCertManager)
	if err != nil {
		return r.fail(fmt.Sprintf("failed to set up %s test: %s", CryostatRecordingTestName, err.Error()))
	}
	defer r.cleanupAndLogs()

	// Create a default Cryostat CR
	cr, err := r.createAndWaitTillCryostatAvailable(r.newCryostatCR())
	if err != nil {
		return r.fail(fmt.Sprintf("failed to determine application URL: %s", err.Error()))
	}
	err = r.StartLogs(cr)
	if err != nil {
		r.Log += fmt.Sprintf("failed to retrieve logs for the application: %s", err.Error())
	}

	base, err := url.Parse(cr.Status.ApplicationURL)
	if err != nil {
		return r.fail(fmt.Sprintf("application URL is invalid: %s", err.Error()))
	}

	err = r.waitTillCryostatReady(base)
	if err != nil {
		return r.fail(fmt.Sprintf("failed to reach the application: %s", err.Error()))
	}

	apiClient := NewCryostatRESTClientset(base)

	// Create a custom target for test
	targetOptions := &Target{
		ConnectUrl: "service:jmx:rmi:///jndi/rmi://localhost:0/jmxrmi",
		Alias:      "customTarget",
	}
	target, err := apiClient.Targets().Create(context.Background(), targetOptions)
	if err != nil {
		return r.fail(fmt.Sprintf("failed to create a target: %s", err.Error()))
	}
	r.Log += fmt.Sprintf("created a custom target: %+v\n", target)

	jmxSecretName := CryostatRecordingTestName + "-jmx-auth"
	secret, err := r.Client.CoreV1().Secrets(namespace).Get(context.Background(), jmxSecretName, metav1.GetOptions{})
	if err != nil {
		return r.fail(fmt.Sprintf("failed to get jmx credentials: %s", err.Error()))
	}

	credential := &Credential{
		UserName:        string(secret.Data["CRYOSTAT_RJMX_USER"]),
		Password:        string(secret.Data["CRYOSTAT_RJMX_PASS"]),
		MatchExpression: fmt.Sprintf("target.alias==\"%s\"", target.Alias),
	}

	err = apiClient.CredentialClient.Create(context.Background(), credential)
	if err != nil {
		return r.fail(fmt.Sprintf("failed to create stored credential: %s", err.Error()))
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
	rec, err := apiClient.Recordings().Create(context.Background(), target, options)
	if err != nil {
		return r.fail(fmt.Sprintf("failed to create a recording: %s", err.Error()))
	}
	r.Log += fmt.Sprintf("created a recording: %+v\n", rec)

	// View the current recording list after creating one
	recs, err := apiClient.Recordings().List(context.Background(), target)
	if err != nil {
		return r.fail(fmt.Sprintf("failed to list recordings: %s", err.Error()))
	}
	r.Log += fmt.Sprintf("current list of recordings: %+v\n", recs)

	// Allow the recording to run for 30s
	time.Sleep(30 * time.Second)

	// Archive the recording
	archiveName, err := apiClient.Recordings().Archive(context.Background(), target, rec.Id)
	if err != nil {
		return r.fail(fmt.Sprintf("failed to archive the recording: %s", err.Error()))
	}
	r.Log += fmt.Sprintf("archived the recording %s at: %s\n", rec.Name, archiveName)

	archives, err := apiClient.Recordings().ListArchives(context.Background(), target)
	if err != nil {
		return r.fail(fmt.Sprintf("failed to list archives: %s", err.Error()))
	}
	r.Log += fmt.Sprintf("current list of archives: %+v\n", archives)

	report, err := apiClient.Recordings().GenerateReport(context.Background(), target, rec)
	if err != nil {
		return r.fail(fmt.Sprintf("failed to generate report for the recording: %s", err.Error()))
	}
	r.Log += fmt.Sprintf("generated report for the recording %s: %+v\n", rec.Name, report)

	// Stop the recording
	err = apiClient.Recordings().Stop(context.Background(), target, rec.Id)
	if err != nil {
		return r.fail(fmt.Sprintf("failed to stop the recording %s: %s", rec.Name, err.Error()))
	}
	// Get the recording to verify its state
	rec, err = apiClient.Recordings().Get(context.Background(), target, rec.Name)
	if err != nil {
		return r.fail(fmt.Sprintf("failed to get the recordings: %s", err.Error()))
	}
	if rec.State != "STOPPED" {
		return r.fail(fmt.Sprintf("recording %s failed to stop: %s", rec.Name, err.Error()))
	}
	r.Log += fmt.Sprintf("stopped the recording: %s\n", rec.Name)

	// Delete the recording
	err = apiClient.Recordings().Delete(context.Background(), target, rec.Id)
	if err != nil {
		return r.fail(fmt.Sprintf("failed to delete the recording %s: %s", rec.Name, err.Error()))
	}
	r.Log += fmt.Sprintf("deleted the recording: %s\n", rec.Name)

	// View the current recording list after deleting one
	recs, err = apiClient.Recordings().List(context.Background(), target)
	if err != nil {
		return r.fail(fmt.Sprintf("failed to list recordings: %s", err.Error()))
	}
	r.Log += fmt.Sprintf("current list of recordings: %+v\n", recs)

	return r.TestResult
}

// CryostatReportTest checks that the operator deploys a report sidecar in response to a Cryostat CR
func CryostatReportTest(bundle *apimanifests.Bundle, namespace string, openShiftCertManager bool) *scapiv1alpha3.TestResult {
	r := newTestResources(CryostatReportTestName, namespace)

	err := r.setupCRTestResources(openShiftCertManager)
	if err != nil {
		return r.fail(fmt.Sprintf("failed to set up %s test: %s", CryostatReportTestName, err.Error()))
	}
	defer r.cleanupAndLogs()

	port := int32(10000)
	cr := r.newCryostatCR()
	cr.Spec.ReportOptions = &operatorv1beta2.ReportConfiguration{
		Replicas: 1,
	}
	cr.Spec.ServiceOptions = &operatorv1beta2.ServiceConfigList{
		ReportsConfig: &operatorv1beta2.ReportsServiceConfig{
			HTTPPort: &port,
		},
	}

	// Create a default Cryostat CR
	_, err = r.createAndWaitTillCryostatAvailable(cr)
	if err != nil {
		return r.fail(fmt.Sprintf("%s test failed: %s", CryostatReportTestName, err.Error()))
	}

	// Query health of report sidecar
	err = r.waitTillReportReady(port)
	if err != nil {
		return r.fail(fmt.Sprintf("failed to reach the application: %s", err.Error()))
	}

	err = r.StartLogs(cr)
	if err != nil {
		r.Log += fmt.Sprintf("failed to retrieve logs for the application: %s", err.Error())
	}

	return r.TestResult
}
