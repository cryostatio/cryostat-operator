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
)

func (r *TestResources) recordingFlow(target *Target, apiClient *CryostatRESTClientset) error {
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
		return fmt.Errorf("failed to create a recording: %s", err.Error())
	}
	r.Log += fmt.Sprintf("created a recording: %+v\n", rec)

	// View the current recording list after creating one
	recs, err := apiClient.Recordings().List(context.Background(), target)
	if err != nil {
		return fmt.Errorf("failed to list recordings: %s", err.Error())
	}
	r.Log += fmt.Sprintf("current list of recordings: %+v\n", recs)

	// Allow the recording to run for 30s
	time.Sleep(30 * time.Second)

	// Archive the recording
	archiveName, err := apiClient.Recordings().Archive(context.Background(), target, rec.Id)
	if err != nil {
		return fmt.Errorf("failed to archive the recording: %s", err.Error())
	}
	r.Log += fmt.Sprintf("archived the recording %s at: %s\n", rec.Name, archiveName)

	archives, err := apiClient.Recordings().ListArchives(context.Background(), target)
	if err != nil {
		return fmt.Errorf("failed to list archives: %s", err.Error())
	}
	r.Log += fmt.Sprintf("current list of archives: %+v\n", archives)

	report, err := apiClient.Recordings().GenerateReport(context.Background(), target, rec)
	if err != nil {
		return fmt.Errorf("failed to generate report for the recording: %s", err.Error())
	}
	r.Log += fmt.Sprintf("generated report for the recording %s: %+v\n", rec.Name, report)

	// Stop the recording
	err = apiClient.Recordings().Stop(context.Background(), target, rec.Id)
	if err != nil {
		return fmt.Errorf("failed to stop the recording %s: %s", rec.Name, err.Error())
	}
	// Get the recording to verify its state
	rec, err = apiClient.Recordings().Get(context.Background(), target, rec.Name)
	if err != nil {
		return fmt.Errorf("failed to get the recordings: %s", err.Error())
	}
	if rec.State != "STOPPED" {
		return fmt.Errorf("recording %s failed to stop: %s", rec.Name, err.Error())
	}
	r.Log += fmt.Sprintf("stopped the recording: %s\n", rec.Name)

	// Delete the recording
	err = apiClient.Recordings().Delete(context.Background(), target, rec.Id)
	if err != nil {
		return fmt.Errorf("failed to delete the recording %s: %s", rec.Name, err.Error())
	}
	r.Log += fmt.Sprintf("deleted the recording: %s\n", rec.Name)

	// View the current recording list after deleting one
	recs, err = apiClient.Recordings().List(context.Background(), target)
	if err != nil {
		return fmt.Errorf("failed to list recordings: %s", err.Error())
	}
	r.Log += fmt.Sprintf("current list of recordings: %+v\n", recs)

	return nil
}
