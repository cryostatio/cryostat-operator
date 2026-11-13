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

// Package cryostatapi models the shapes of Cryostat's own REST and GraphQL APIs: request
// bodies, response payloads, and the small helpers (form encoding, GraphQL error inspection)
// that go with them. It has no dependency on Kubernetes, the operator, or any particular test
// framework, so it can be reused by any scorecard test, or any other Go code, that needs to
// talk to a running Cryostat instance.
package cryostatapi

import (
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"strings"
)

const graphQLForbiddenMessage = "Forbidden"

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
	RemoteId    uint32 `json:"remoteId"`
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

// GraphQLError is a single entry of a GraphQL response's "errors" array.
type GraphQLError struct {
	Message string `json:"message"`
}

// GraphQLErrors is the error portion of any GraphQL response, shared by the response types
// below so that permission denials can be detected without modelling the "data" payload.
type GraphQLErrors struct {
	Errors []GraphQLError `json:"errors,omitempty"`
}

// Denied reports whether the response carries a permission denial raised by Cryostat's
// GraphQL security interceptor.
func (r *GraphQLErrors) Denied() bool {
	return r.ErrorMatching(graphQLForbiddenMessage) != nil
}

// ErrorMatching returns the first error whose message contains the given substring, or nil.
func (r *GraphQLErrors) ErrorMatching(substring string) *GraphQLError {
	for i := range r.Errors {
		if strings.Contains(r.Errors[i].Message, substring) {
			return &r.Errors[i]
		}
	}
	return nil
}

// Messages returns all error messages, for inclusion in assertion failure output.
func (r *GraphQLErrors) Messages() []string {
	messages := make([]string, 0, len(r.Errors))
	for _, e := range r.Errors {
		messages = append(messages, e.Message)
	}
	return messages
}

type ArchiveGraphQLResponse struct {
	GraphQLErrors
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

// ArchivedRecordingsGraphQLResponse models the top-level "archivedRecordings" query, which
// requires only archivedrecordings:read. The nested doPutMetadata and doDelete fields are
// resolvers on the ArchivedRecording type and are guarded by archivedrecordings:write and
// archivedrecordings:delete respectively.
type ArchivedRecordingsGraphQLResponse struct {
	GraphQLErrors
	Data struct {
		ArchivedRecordings struct {
			Data []ArchivedRecordingNode `json:"data"`
		} `json:"archivedRecordings"`
	} `json:"data"`
}

// ArchivedRecordingNode is an archived recording as returned by the top-level
// "archivedRecordings" GraphQL query, including the results of mutation resolvers nested
// underneath it (such as doPutMetadata and doDelete).
type ArchivedRecordingNode struct {
	Name          string   `json:"name"`
	JvmId         string   `json:"jvmId"`
	DoPutMetadata *Archive `json:"doPutMetadata,omitempty"`
	DoDelete      *Archive `json:"doDelete,omitempty"`
}

// ArchivedThreadDumpDirectory is one target's thread dumps, as returned by
// GET /api/beta/diagnostics/fs/threaddumps.
type ArchivedThreadDumpDirectory struct {
	JvmId       string            `json:"jvmId"`
	ThreadDumps []ThreadDumpEntry `json:"threadDumps"`
}

type ThreadDumpEntry struct {
	JvmId        string `json:"jvmId"`
	ThreadDumpId string `json:"threadDumpId"`
}

// ArchivedHeapDumpDirectory is one target's heap dumps, as returned by
// GET /api/beta/diagnostics/fs/heapdumps.
type ArchivedHeapDumpDirectory struct {
	JvmId     string          `json:"jvmId"`
	HeapDumps []HeapDumpEntry `json:"heapDumps"`
}

type HeapDumpEntry struct {
	JvmId      string `json:"jvmId"`
	HeapDumpId string `json:"heapDumpId"`
}

// ArchivedUnifiedLogDirectory is one target's unified logs, as returned by
// GET /api/beta/diagnostics/fs/unified-logs.
type ArchivedUnifiedLogDirectory struct {
	JvmId       string            `json:"jvmId"`
	UnifiedLogs []UnifiedLogEntry `json:"unifiedLogs"`
}

type UnifiedLogEntry struct {
	JvmId string `json:"jvmId"`
	LogId string `json:"logId"`
}

// UnifiedLogSession is the response to enabling or pulling a unified logging session, as
// returned by POST .../unified-logging and POST .../unified-logging/pull.
type UnifiedLogSession struct {
	JvmId string `json:"jvmId"`
	LogId string `json:"logId"`
}

// Rule is an automated rule, as returned by the /api/v4/rules endpoints.
type Rule struct {
	Name string `json:"name"`
}

// CredentialResponse is a stored credential as returned by POST /api/v4/credentials.
type CredentialResponse struct {
	Id uint32 `json:"id"`
}

// EventTemplate is a custom event template as returned by the /api/v4/event_templates
// endpoints.
type EventTemplate struct {
	Name string `json:"name"`
}
