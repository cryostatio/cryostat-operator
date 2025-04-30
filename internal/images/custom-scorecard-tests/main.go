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

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"slices"
	"strings"

	scapiv1alpha3 "github.com/operator-framework/api/pkg/apis/scorecard/v1alpha3"
	apimanifests "github.com/operator-framework/api/pkg/manifests"

	tests "github.com/cryostatio/cryostat-operator/internal/test/scorecard"
)

const podBundleRoot = "/bundle"

const argInstallOpenShiftCertManager = "installOpenShiftCertManager"

const argListTests = "list"

func main() {
	openShiftCertManager := flag.Bool(argInstallOpenShiftCertManager, false, "installs the cert-manager Operator for Red Hat OpenShift")
	listTests := flag.Bool(argListTests, false, "list available test names")
	flag.Parse()
	if openShiftCertManager == nil {
		// Default to false
		openShiftCertManager = &[]bool{false}[0]
	}
	if listTests == nil {
		listTests = &[]bool{false}[0]
	}

	if *listTests {
		log.Printf("available tests: %s\nuse '*' or 'ALL' to select all", strings.Join(validTests(), ", "))
	}

	entrypoint := flag.Args()
	if len(entrypoint) == 0 {
		log.Fatal("specify one or more test name arguments (separated by whitespace), or '*' or 'ALL' to select all")
	} else if len(entrypoint) == 1 && (entrypoint[0] == "*" || entrypoint[0] == "ALL") {
		entrypoint = validTests()
	}

	// Get namespace from SCORECARD_NAMESPACE environment variable
	namespace := os.Getenv("SCORECARD_NAMESPACE")
	if len(namespace) == 0 {
		log.Fatal("SCORECARD_NAMESPACE environment variable not set")
	}

	// Get the path to the bundle from BUNDLE_DIR environment variable
	// If empty, assume running within a pod and use a well-known path to the pod's untar'd bundle
	bundleDir := os.Getenv("BUNDLE_DIR")
	if len(bundleDir) == 0 {
		bundleDir = podBundleRoot
	}

	// Read the bundle from the specified path
	bundle, err := apimanifests.GetBundleFromDir(bundleDir)
	if err != nil {
		log.Fatalf("failed to read bundle manifest: %s", err.Error())
	}

	var results []scapiv1alpha3.TestResult

	// Check that test arguments are valid
	if !validateTests(entrypoint) {
		results = printValidTests(entrypoint)
	} else {
		results = runTests(entrypoint, bundle, namespace, *openShiftCertManager)
	}

	// Print results in expected JSON form
	printJSONResults(results)
}

type testFn func(*apimanifests.Bundle, string, bool) *scapiv1alpha3.TestResult

func testLookup() map[string]testFn {
	m := make(map[string]testFn)
	m[tests.OperatorInstallTestName] = tests.OperatorInstallTest
	m[tests.CryostatCRTestName] = tests.CryostatCRTest
	m[tests.CryostatMultiNamespaceTestName] = tests.CryostatMultiNamespaceTest
	m[tests.CryostatRecordingTestName] = tests.CryostatRecordingTest
	m[tests.CryostatConfigChangeTestName] = tests.CryostatConfigChangeTest
	m[tests.CryostatReportTestName] = tests.CryostatReportTest
	return m
}

func validTests() []string {
	tests := testLookup()
	keys := []string{}
	for k := range tests {
		keys = append(keys, k)
	}
	return keys
}

func printValidTests(requests []string) []scapiv1alpha3.TestResult {
	result := scapiv1alpha3.TestResult{}
	result.State = scapiv1alpha3.FailState
	result.Errors = make([]string, 0)
	result.Suggestions = make([]string, 0)

	str := fmt.Sprintf(
		"invalid tests requested: %s. valid tests for this image include: %s",
		strings.Join(requests, ","),
		strings.Join(validTests(), ","),
	)
	result.Errors = append(result.Errors, str)

	return []scapiv1alpha3.TestResult{result}
}

func validateTests(testNames []string) bool {
	for _, testName := range testNames {
		if !slices.Contains(validTests(), testName) {
			return false
		}
	}
	return true
}

func testNamed(testName string) testFn {
	fn, ok := testLookup()[testName]
	if !ok {
		panic(fmt.Sprintf("test name \"%s\" could not be resolved", testName))
	}
	return fn
}

func runTests(testNames []string, bundle *apimanifests.Bundle, namespace string,
	openShiftCertManager bool) []scapiv1alpha3.TestResult {
	results := []scapiv1alpha3.TestResult{}

	// Run tests
	for _, testName := range testNames {
		results = append(results, *testNamed(testName)(bundle, namespace, openShiftCertManager))
	}
	return results
}

func printJSONResults(results []scapiv1alpha3.TestResult) {
	status := scapiv1alpha3.TestStatus{
		Results: results,
	}
	prettyJSON, err := json.MarshalIndent(status, "", "    ")
	if err != nil {
		log.Fatal("failed to generate json", err)
	}
	fmt.Printf("%s\n", string(prettyJSON))
}
