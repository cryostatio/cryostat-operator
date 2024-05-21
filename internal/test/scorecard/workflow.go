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
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (r *TestResources) recordingFlow(target *Target, apiClient *CryostatRESTClientset, ctx context.Context) error {
	jmxSecretName := r.Name + "-jmx-auth"
	secret, err := r.Client.CoreV1().Secrets(r.Namespace).Get(ctx, jmxSecretName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get jmx credentials: %s", err.Error())
	}

	credential := &Credential{
		UserName:        string(secret.Data["CRYOSTAT_RJMX_USER"]),
		Password:        string(secret.Data["CRYOSTAT_RJMX_PASS"]),
		MatchExpression: fmt.Sprintf("target.alias==\"%s\"", target.Alias),
	}

	err = apiClient.CredentialClient.Create(ctx, credential)
	if err != nil {
		return fmt.Errorf("failed to create stored credential: %s", err.Error())
	}
	r.Log += fmt.Sprintf("created stored credential with match expression: %s\n", credential.MatchExpression)

	// Wait for Cryostat to update the discovery tree
	time.Sleep(2 * time.Second)

	connectUrl := target.ConnectUrl
	// Create a recording
	options := &RecordingCreateOptions{
		RecordingName: "scorecard_test_rec",
		Events:        "template=ALL",
		Duration:      0, // Continuous
		ToDisk:        true,
		MaxSize:       0,
		MaxAge:        0,
	}
	rec, err := apiClient.Recordings().Create(ctx, connectUrl, options)
	if err != nil {
		return fmt.Errorf("failed to create a recording: %s", err.Error())
	}
	r.Log += fmt.Sprintf("created a recording: %+v\n", rec)

	// View the current recording list after creating one
	recs, err := apiClient.Recordings().List(ctx, connectUrl)
	if err != nil {
		return fmt.Errorf("failed to list recordings: %s", err.Error())
	}
	r.Log += fmt.Sprintf("current list of recordings: %+v\n", recs)

	// Allow the recording to run for 10s
	time.Sleep(30 * time.Second)

	// Archive the recording
	archiveName, err := apiClient.Recordings().Archive(ctx, connectUrl, rec.Name)
	if err != nil {
		return fmt.Errorf("failed to archive the recording: %s", err.Error())

	}
	r.Log += fmt.Sprintf("archived the recording %s at: %s\n", rec.Name, archiveName)

	archives, err := apiClient.Recordings().ListArchives(ctx, connectUrl)
	if err != nil {
		return fmt.Errorf("failed to list archives: %s", err.Error())
	}
	r.Log += fmt.Sprintf("current list of archives: %+v\n", archives)

	report, err := apiClient.Recordings().GenerateReport(ctx, connectUrl, rec)
	if err != nil {
		return fmt.Errorf("failed to generate report for the recording: %s", err.Error())
	}
	r.Log += fmt.Sprintf("generated report for the recording %s: %+v\n", rec.Name, report)

	// Stop the recording
	err = apiClient.Recordings().Stop(ctx, connectUrl, rec.Name)
	if err != nil {
		return fmt.Errorf("failed to stop the recording %s: %s", rec.Name, err.Error())
	}
	// Get the recording to verify its state
	rec, err = apiClient.Recordings().Get(ctx, connectUrl, rec.Name)
	if err != nil {
		return fmt.Errorf("failed to get the recordings: %s", err.Error())
	}
	if rec.State != "STOPPED" {
		return fmt.Errorf("recording %s failed to stop: %s", rec.Name, err.Error())
	}
	r.Log += fmt.Sprintf("stopped the recording: %s\n", rec.Name)

	// Delete the recording
	err = apiClient.Recordings().Delete(ctx, connectUrl, rec.Name)
	if err != nil {
		return fmt.Errorf("failed to delete the recording %s: %s", rec.Name, err.Error())
	}
	r.Log += fmt.Sprintf("deleted the recording: %s\n", rec.Name)

	// View the current recording list after deleting one
	recs, err = apiClient.Recordings().List(ctx, connectUrl)
	if err != nil {
		return fmt.Errorf("failed to list recordings: %s", err.Error())
	}
	r.Log += fmt.Sprintf("current list of recordings: %+v\n", recs)

	return nil
}
