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
	"fmt"
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

type CustomTargetResponse struct {
	Data struct {
		Result *Target `json:"result"`
	} `json:"data"`
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

const (
	GRAFANA_DASHBOARD_UID     = "main"
	GRAFANA_DASHBOARD_TITLE   = "Cryostat Dashboard"
	GRAFANA_DATASOURCE_NAME   = "jfr-datasource"
	GRAFANA_DATASOURCE_TYPE   = "grafana-simple-json-datasource"
	GRAFANA_DATASOURCE_ACCESS = "proxy"
)

// DataSource represents a Grafana data source
type DataSource struct {
	ID   uint32 `json:"id"`
	UID  string `json:"uid"`
	Name string `json:"name"`

	Type string `json:"type"`

	URL    string `json:"url"`
	Access string `json:"access"`

	BasicAuth bool `json:"basicAuth"`
}

func (ds *DataSource) Valid() error {
	if ds.Name != GRAFANA_DATASOURCE_NAME {
		return fmt.Errorf("expected datasource name %s, but got %s", GRAFANA_DATASOURCE_NAME, ds.Name)
	}

	if ds.Type != GRAFANA_DATASOURCE_TYPE {
		return fmt.Errorf("expected datasource type %s, but got %s", GRAFANA_DATASOURCE_TYPE, ds.Type)
	}

	if len(ds.URL) == 0 {
		return errors.New("expected datasource url, but got empty")
	}

	if ds.Access != GRAFANA_DATASOURCE_ACCESS {
		return fmt.Errorf("expected datasource access mode %s, but got %s", GRAFANA_DATASOURCE_ACCESS, ds.Access)
	}

	if ds.BasicAuth {
		return errors.New("expected basicAuth to be disabled, but got enabled")
	}

	return nil
}

// DashBoard represents a Grafana dashboard
type DashBoard struct {
	DashBoardMeta `json:"meta"`
	DashBoardInfo `json:"dashboard"`
}

type DashBoardMeta struct {
	Slug        string `json:"slug"`
	URL         string `json:"url"`
	Provisioned bool   `json:"provisioned"`
}

type DashBoardInfo struct {
	UID         string                 `json:"uid"`
	Title       string                 `json:"title"`
	Annotations map[string]interface{} `json:"annotations"`
	Panels      []Panel                `json:"panels"`
}

// Panel represents a Grafana panel.
// A panel can be used either for displaying data or separating groups
type Panel struct {
	ID      uint32       `json:"id"`
	Title   string       `json:"title"`
	Type    string       `json:"type"`
	Targets []PanelQuery `json:"targets"`
	Panels  []Panel      `json:"panels"`
}

type PanelQuery struct {
	RawQuery bool   `json:"rawQuery"`
	RefID    string `json:"refId"`
	Target   string `json:"target"`
	Type     string `json:"table"`
}

func (db *DashBoard) Valid() error {
	if db.UID != GRAFANA_DASHBOARD_UID {
		return fmt.Errorf("expected dashboard uid %s, but got %s", GRAFANA_DASHBOARD_UID, db.UID)
	}

	if db.Title != GRAFANA_DASHBOARD_TITLE {
		return fmt.Errorf("expected dashboard title %s, but got %s", GRAFANA_DASHBOARD_TITLE, db.Title)
	}

	if !db.Provisioned {
		return errors.New("expected dashboard to be provisioned, but got unprovisioned")
	}

	if len(db.Panels) == 0 {
		return errors.New("expected dashboard to have panels, but got 0")
	}

	return nil
}
