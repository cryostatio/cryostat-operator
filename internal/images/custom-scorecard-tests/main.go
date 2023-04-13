// Copyright The Cryostat Authors
//
// The Universal Permissive License (UPL), Version 1.0
//
// Subject to the condition set forth below, permission is hereby granted to any
// person obtaining a copy of this software, associated documentation and/or data
// (collectively the "Software"), free of charge and under any and all copyright
// rights in the Software, and any and all patent rights owned or freely
// licensable by each licensor hereunder covering either (i) the unmodified
// Software as contributed to or provided by such licensor, or (ii) the Larger
// Works (as defined below), to deal in both
//
// (a) the Software, and
// (b) any piece of software and/or hardware listed in the lrgrwrks.txt file if
// one is included with the Software (each a "Larger Work" to which the Software
// is contributed by such licensors),
//
// without restriction, including without limitation the rights to copy, create
// derivative works of, display, perform, and distribute the Software and make,
// use, sell, offer for sale, import, export, have made, and have sold the
// Software and the Larger Work(s), and to sublicense the foregoing rights on
// either these or other terms.
//
// This license is subject to the following condition:
// The above copyright notice and either this complete permission notice or at
// a minimum a reference to the UPL must be included in all copies or
// substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	scapiv1alpha3 "github.com/operator-framework/api/pkg/apis/scorecard/v1alpha3"
	apimanifests "github.com/operator-framework/api/pkg/manifests"

	tests "github.com/cryostatio/cryostat-operator/internal/test/scorecard"
)

// const podBundleRoot = "/bundle"
const podBundleRoot = "../../../../bundle" // FIXME

const argInstallOpenShiftCertManager = "installOpenShiftCertManager"

func main() {
	openShiftCertManager := flag.Bool(argInstallOpenShiftCertManager, false, "installs the cert-manager Operator for Red Hat OpenShift")
	flag.Parse()
	if openShiftCertManager == nil {
		openShiftCertManager = &[]bool{false}[0]
	}

	entrypoint := flag.Args()
	if len(entrypoint) == 0 {
		log.Fatal("specify one or more test name arguments")
	}
	fmt.Println(entrypoint)

	// Get namespace from SCORECARD_NAMESPACE environment variable
	namespace := os.Getenv("SCORECARD_NAMESPACE")
	if len(namespace) == 0 {
		log.Fatal("SCORECARD_NAMESPACE environment variable not set")
	}

	// Read the pod's untar'd bundle from a well-known path.
	bundle, err := apimanifests.GetBundleFromDir(podBundleRoot)
	if err != nil {
		log.Fatalf("failed to read bundle manifest: %s", err.Error())
	}

	var results []scapiv1alpha3.TestResult

	// Check that test arguments are valid
	if !validateTests(entrypoint) {
		results = printValidTests()
	} else {
		results = runTests(entrypoint, bundle, namespace, *openShiftCertManager)
	}

	// Print results in expected JSON form
	printJSONResults(results)
}

func printValidTests() []scapiv1alpha3.TestResult {
	result := scapiv1alpha3.TestResult{}
	result.State = scapiv1alpha3.FailState
	result.Errors = make([]string, 0)
	result.Suggestions = make([]string, 0)

	str := fmt.Sprintf("valid tests for this image include: %s", strings.Join([]string{
		tests.OperatorInstallTestName,
		tests.CryostatCRTestName,
	}, ","))
	result.Errors = append(result.Errors, str)

	return []scapiv1alpha3.TestResult{result}
}

func validateTests(testNames []string) bool {
	for _, testName := range testNames {
		switch testName {
		case tests.OperatorInstallTestName:
		case tests.CryostatCRTestName:
		default:
			return false
		}
	}
	return true
}

func runTests(testNames []string, bundle *apimanifests.Bundle, namespace string,
	openShiftCertManager bool) []scapiv1alpha3.TestResult {
	results := []scapiv1alpha3.TestResult{}

	// Run tests
	for _, testName := range testNames {
		switch testName {
		case tests.OperatorInstallTestName:
			results = append(results, tests.OperatorInstallTest(bundle, namespace))
		case tests.CryostatCRTestName:
			results = append(results, tests.CryostatCRTest(bundle, namespace, openShiftCertManager))
		default:
			log.Fatalf("unknown test found: %s", testName)
		}
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
