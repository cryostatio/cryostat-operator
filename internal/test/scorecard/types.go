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

import "github.com/cryostatio/cryostat-operator/internal/cryostatapi"

type HealthResponse = cryostatapi.HealthResponse

type RecordingCreateOptions = cryostatapi.RecordingCreateOptions

type Credential = cryostatapi.Credential

type Recording = cryostatapi.Recording

type Archive = cryostatapi.Archive

type Target = cryostatapi.Target

type GraphQLQuery = cryostatapi.GraphQLQuery

type GraphQLError = cryostatapi.GraphQLError

type GraphQLErrors = cryostatapi.GraphQLErrors

type ArchiveGraphQLResponse = cryostatapi.ArchiveGraphQLResponse

type ArchivedRecordingsGraphQLResponse = cryostatapi.ArchivedRecordingsGraphQLResponse

type ArchivedRecordingNode = cryostatapi.ArchivedRecordingNode

type ArchivedThreadDumpDirectory = cryostatapi.ArchivedThreadDumpDirectory

type ThreadDumpEntry = cryostatapi.ThreadDumpEntry

type ArchivedHeapDumpDirectory = cryostatapi.ArchivedHeapDumpDirectory

type HeapDumpEntry = cryostatapi.HeapDumpEntry

type ArchivedUnifiedLogDirectory = cryostatapi.ArchivedUnifiedLogDirectory

type UnifiedLogEntry = cryostatapi.UnifiedLogEntry

type UnifiedLogSession = cryostatapi.UnifiedLogSession

type Rule = cryostatapi.Rule
