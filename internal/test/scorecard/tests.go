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
)

const (
	OperatorInstallTestName         string = "operator-install"
	ConsolePluginTestName           string = "console-plugin"
	CryostatCRTestName              string = "cryostat-cr"
	CryostatMultiNamespaceTestName  string = "cryostat-multi-namespace"
	CryostatRecordingTestName       string = "cryostat-recording"
	CryostatAgentTestName           string = "cryostat-agent"
	CryostatAgentAutoConfigTestName string = "cryostat-agent-autoconfig"
	CryostatConfigChangeTestName    string = "cryostat-config-change"
	CryostatReportTestName          string = "cryostat-report"
	CryostatRBACTestName            string = "cryostat-rbac"
)

// rbacSkipReason explains why an OpenShift-only fine-grained RBAC test cannot carry signal
// on Kubernetes. It is logged verbatim in the skip branch so that a green run on a
// non-OpenShift cluster is not mistaken for passing RBAC coverage.
func rbacSkipReason(testName string) string {
	return fmt.Sprintf("skipping %s: Cryostat's fine-grained RBAC is only enforced"+
		" when deployed on OpenShift. This test always passes on non-OpenShift Kubernetes since"+
		" Cryostat will trivially pass all RBAC lookups as noops.", testName)
}

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
	cr.Spec.StorageOptions = &operatorv1beta2.StorageConfigurations{
		Database: &operatorv1beta2.StorageConfiguration{
			EmptyDir: &operatorv1beta2.EmptyDirConfig{
				Enabled: true,
			},
		},
		ObjectStorage: &operatorv1beta2.StorageConfiguration{
			EmptyDir: &operatorv1beta2.EmptyDirConfig{
				Enabled: true,
			},
		},
	}

	cr, err = r.createAndWaitTillCryostatAvailable(cr)
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

	apiClient := r.NewCryostatRESTClientset(base)

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

	// Wait for Cryostat to update the discovery tree.
	time.Sleep(2 * time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	if err := r.exerciseRecording(ctx, apiClient, target); err != nil {
		return r.fail(err.Error())
	}

	return r.TestResult
}

// CryostatRBACTest checks that the fine-grained RBAC permission mappings configured on a
// Cryostat CR are enforced by the deployed Cryostat application.
//
// Four ServiceAccounts form a monotone privilege ladder over archived recordings - none,
// read, read+write, read+write+delete - expressed as Kubernetes Roles over synthetic
// resources that the CR's rbacPermissions map onto Cryostat's archivedrecordings
// permissions. Each ServiceAccount then exercises all three permissions against both the
// REST and the GraphQL surface.
//
// The test is a no-op on Kubernetes: see rbacSkipReason.
func CryostatRBACTest(bundle *apimanifests.Bundle, namespace string, openShiftCertManager bool) *scapiv1alpha3.TestResult {
	r := newTestResources(CryostatRBACTestName, namespace)

	err := r.setupCRTestResources(openShiftCertManager)
	if err != nil {
		return r.fail(fmt.Sprintf("failed to set up %s test: %s", CryostatRBACTestName, err.Error()))
	}
	if !r.OpenShift {
		r.Log += rbacSkipReason(CryostatRBACTestName) + "\n"
		return r.TestResult
	}
	defer r.cleanupAndLogs()

	setupCtx, cancelSetup := context.WithTimeout(context.Background(), testTimeout)
	defer cancelSetup()

	levels := rbacPrivilegeLevels()
	defer r.cleanupRBACSubjects(levels)
	err = r.setupRBACSubjects(setupCtx, rbacArchivesResource, archivedRecordingsSupportingRules(), levels)
	if err != nil {
		return r.fail(fmt.Sprintf("failed to set up RBAC subjects: %s", err.Error()))
	}

	cr, err := r.createAndWaitTillCryostatAvailable(r.newRBACCryostatCR(archivedRecordingsRBACPermissions()))
	if err != nil {
		return r.fail(fmt.Sprintf("%s test failed: %s", CryostatRBACTestName, err.Error()))
	}
	err = r.StartLogs(cr)
	if err != nil {
		r.Log += fmt.Sprintf("failed to retrieve logs for the application: %s\n", err.Error())
	}

	base, err := url.Parse(cr.Status.ApplicationURL)
	if err != nil {
		return r.fail(fmt.Sprintf("application URL is invalid: %s", err.Error()))
	}
	err = r.waitTillCryostatReady(base)
	if err != nil {
		return r.fail(fmt.Sprintf("failed to reach the application: %s", err.Error()))
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	fixture, err := r.setupRBACFixture(ctx, cr, base)
	if err != nil {
		return r.fail(fmt.Sprintf("failed to set up recordings for %s test: %s", CryostatRBACTestName, err.Error()))
	}

	r.assertRBACMatrix(ctx, base, fixture, rbacArchivesResource, levels, archivedRecordingsAssertions())

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
		Replicas:  1,
		Resources: r.newBasicReportResourceConfig(),
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
