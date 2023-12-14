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
	"net/url"
	"strconv"
)

type RecordingCreateOptions struct {
	RecordingName string
	Events        string
	Duration      int32
	ToDisk        bool
	MaxSize       int32
	MaxAge        int32
}

func (opts *RecordingCreateOptions) ToFormData() string {
	formData := &url.Values{}

	formData.Add("recordingName", url.PathEscape(opts.RecordingName))
	formData.Add("events", opts.Events)
	formData.Add("duration", strconv.Itoa(int(opts.Duration)))
	formData.Add("toDisk", strconv.FormatBool(opts.ToDisk))
	formData.Add("maxSize", strconv.Itoa(int(opts.MaxSize)))
	formData.Add("maxAge", strconv.Itoa(int(opts.MaxAge)))

	return formData.Encode()
}

type Recording struct {
	DownloadURL string `json:"downloadUrl"`
	ReportURL   string `json:"reportUrl"`
	Id          string `json:"id"`
	Name        string `json:"name"`
	StartTime   int32  `json:"startTime"`
	State       string `json:"state"`
	Duration    int32  `json:"duration"`
	Continuous  bool   `json:"continuous"`
	ToDisk      bool   `json:"toDisk"`
	MaxSize     int32  `json:"maxSize"`
	MaxAge      int32  `json:"maxAge"`
}

type Target struct {
	ConnectUrl string `json:"connectUrl"`
	Alias      string `json:"alias,omitempty"`
}
