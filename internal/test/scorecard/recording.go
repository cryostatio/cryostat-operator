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

// exerciseRecording runs the same recording lifecycle for JMX and Agent targets.
func (r *TestResources) exerciseRecording(ctx context.Context, apiClient *CryostatRESTClientset, target *Target) error {
	// Create a recording
	options := &RecordingCreateOptions{
		RecordingName: "scorecard_test_rec",
		Events:        "template=ALL",
		Duration:      0, // Continuous
		ToDisk:        true,
		MaxSize:       0,
		MaxAge:        0,
	}
	rec, err := apiClient.Recordings().Create(ctx, target, options)
	if err != nil {
		return fmt.Errorf("failed to create a recording: %s", err.Error())
	}
	r.Log += fmt.Sprintf("created a recording: %+v\n", rec)

	// View the current recording list after creating one
	recs, err := apiClient.Recordings().List(ctx, target)
	if err != nil {
		return fmt.Errorf("failed to list recordings: %s", err.Error())
	}
	r.Log += fmt.Sprintf("current list of recordings: %+v\n", recs)
	found := false
	for _, recording := range recs {
		if recording.Id == rec.Id && recording.Name == rec.Name && recording.State == "RUNNING" {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("running recording %s not found in recording list", rec.Name)
	}

	// Allow the recording to run for 30s
	select {
	case <-time.After(30 * time.Second):
	case <-ctx.Done():
		return ctx.Err()
	}

	// Archive the recording
	archiveJobID, err := apiClient.Recordings().Archive(ctx, target, rec.Id)
	if err != nil {
		return fmt.Errorf("failed to archive the recording: %s", err.Error())
	}
	r.Log += fmt.Sprintf("recording archival job ID for the recording %s at: %s\n", rec.Name, archiveJobID)

	// TODO: Poll until this recording appears in the archive list, failing on timeout.
	// Archival is asynchronous, so the first list response may still be empty.
	archives, err := apiClient.Recordings().ListArchives(ctx, target)
	if err != nil {
		return fmt.Errorf("failed to list archives: %s", err.Error())
	}
	r.Log += fmt.Sprintf("current list of archives: %+v\n", archives)

	reportJobId, err := apiClient.Recordings().RequestReportGeneration(ctx, target, rec)
	if err != nil {
		return fmt.Errorf("failed to generate report for the recording: %s", err.Error())
	}
	r.Log += fmt.Sprintf("report generation job ID for the recording %s: %+v\n", rec.Name, *reportJobId)

	// Stop the recording
	err = apiClient.Recordings().Stop(ctx, target, rec.Id)
	if err != nil {
		return fmt.Errorf("failed to stop the recording %s: %s", rec.Name, err.Error())
	}
	// Get the recording to verify its state
	rec, err = apiClient.Recordings().Get(ctx, target, rec.Name)
	if err != nil {
		return fmt.Errorf("failed to get the recordings: %s", err.Error())
	}
	if rec.State != "STOPPED" {
		return fmt.Errorf("expect recording %s to have state STOPPED but found %s", rec.Name, rec.State)
	}
	r.Log += fmt.Sprintf("stopped the recording: %s\n", rec.Name)

	// Delete the recording
	err = apiClient.Recordings().Delete(ctx, target, rec.Id)
	if err != nil {
		return fmt.Errorf("failed to delete the recording %s: %s", rec.Name, err.Error())
	}
	r.Log += fmt.Sprintf("deleted the recording: %s\n", rec.Name)

	// View the current recording list after deleting one
	recs, err = apiClient.Recordings().List(ctx, target)
	if err != nil {
		return fmt.Errorf("failed to list recordings: %s", err.Error())
	}
	r.Log += fmt.Sprintf("current list of recordings: %+v\n", recs)

	// Verify that the recording is deleted
	for _, recording := range recs {
		if recording.Id == rec.Id {
			return fmt.Errorf("expect recording %s to be deleted but found %+v", rec.Name, recording)
		}
	}

	return nil
}
