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
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
)

type HealthResponse struct {
	CryostatVersion      string `json:"cryostatVersion"`
	DashboardAvailable   bool   `json:"dashboardAvailable"`
	DashboardConfigured  bool   `json:"dashboardConfigured"`
	DataSourceAvailable  bool   `json:"datasourceAvailable"`
	DataSourceConfigured bool   `json:"datasourceConfigured"`
	ReportsAvailable     bool   `json:"reportsAvailable"`
	ReportsConfigured    bool   `json:"reportsConfigured"`
}

func (health *HealthResponse) Ready() error {
	if !health.DashboardAvailable {
		return errors.New("dashboard is not available")
	}

	if !health.DataSourceAvailable {
		return errors.New("datasource is not available")
	}

	if !health.ReportsAvailable {
		return errors.New("report is not available")
	}
	return nil
}

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

	formData.Add("recordingName", opts.RecordingName)
	formData.Add("events", opts.Events)
	formData.Add("duration", strconv.Itoa(int(opts.Duration)))
	formData.Add("toDisk", strconv.FormatBool(opts.ToDisk))
	formData.Add("maxSize", strconv.Itoa(int(opts.MaxSize)))
	formData.Add("maxAge", strconv.Itoa(int(opts.MaxAge)))

	return formData.Encode()
}

type Credential struct {
	UserName        string
	Password        string
	MatchExpression string
}

func (cred *Credential) ToFormData() string {
	formData := &url.Values{}

	formData.Add("username", cred.UserName)
	formData.Add("password", cred.Password)
	formData.Add("matchExpression", cred.MatchExpression)

	return formData.Encode()
}

type Recording struct {
	DownloadURL string `json:"downloadUrl"`
	ReportURL   string `json:"reportUrl"`
	Id          uint32 `json:"id"`
	Name        string `json:"name"`
	StartTime   uint64 `json:"startTime"`
	State       string `json:"state"`
	Duration    int32  `json:"duration"`
	Continuous  bool   `json:"continuous"`
	ToDisk      bool   `json:"toDisk"`
	MaxSize     int32  `json:"maxSize"`
	MaxAge      int32  `json:"maxAge"`
}

type Archive struct {
	Name        string `json:"name"`
	DownloadUrl string `json:"downloadUrl"`
	ReportUrl   string `json:"reportUrl"`
	Metadata    struct {
		Labels []struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		}
	} `json:"metadata"`
	Size uint32 `json:"size"`
}

type Target struct {
	Id         uint32 `json:"id,omitempty"`
	ConnectUrl string `json:"connectUrl"`
	Alias      string `json:"alias,omitempty"`
}

func (target *Target) ToFormData() string {
	formData := &url.Values{}

	formData.Add("connectUrl", target.ConnectUrl)
	formData.Add("alias", target.Alias)

	return formData.Encode()
}

type GraphQLQuery struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables,omitempty"`
}

func (query *GraphQLQuery) ToJSON() ([]byte, error) {
	return json.Marshal(query)
}

type ArchiveGraphQLResponse struct {
	Data struct {
		TargetNodes []struct {
			Target struct {
				ArchivedRecordings struct {
					Data []Archive `json:"data"`
				} `json:"archivedRecordings"`
			} `json:"target"`
		} `json:"targetNodes"`
	} `json:"data"`
}
