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
	"encoding/json"
	"net/http"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func writeArchives(w http.ResponseWriter, archives []map[string]any) {
	GinkgoHelper()
	Expect(json.NewEncoder(w).Encode(map[string]any{
		"data": map[string]any{"targetNodes": []any{
			map[string]any{"target": map[string]any{"archivedRecordings": map[string]any{"data": archives}}},
		}},
	})).To(Succeed())
}

func testArchive(id string, size uint32) map[string]any {
	return map[string]any{
		"name":     "sample_scorecard_test_rec_20261112T210000Z.jfr",
		"size":     size,
		"metadata": map[string]any{"labels": []any{map[string]string{"key": "activeRecordingId", "value": id}}},
	}
}

var _ = Describe("Recording archival", func() {
	It("waits for a nonempty archive of the requested recording", func() {
		// Return an empty list, then unrelated and empty archives, and finally
		// the requested recording with data. Only the last response may pass.
		var calls atomic.Int32
		api := testCryostatAPI(func(w http.ResponseWriter, request *http.Request) {
			Expect(request.Method).To(Equal(http.MethodPost))
			Expect(request.URL.Path).To(Equal("/api/v4/graphql"))
			var query GraphQLQuery
			Expect(json.NewDecoder(request.Body).Decode(&query)).To(Succeed())
			Expect(query.Variables["id"]).To(Equal(float64(17)))
			archives := []map[string]any{}
			call := calls.Add(1)
			if call >= 2 {
				archives = append(archives, testArchive("99", 1024), testArchive("7", 0))
			}
			if call >= 3 {
				archives = append(archives, testArchive("7", 2048))
			}
			writeArchives(w, archives)
		})
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		archive, err := waitForArchivedRecording(ctx, api.Recordings(), &Target{Id: 17}, &Recording{Id: 7, Name: "scorecard_test_rec"})
		Expect(err).ToNot(HaveOccurred())
		Expect(archive.Size).To(Equal(uint32(2048)))
		Expect(calls.Load()).To(Equal(int32(3)))
	})

	It("fails at the deadline when only an unrelated archive appears", func() {
		api := testCryostatAPI(func(w http.ResponseWriter, _ *http.Request) {
			writeArchives(w, []map[string]any{testArchive("99", 1024)})
		})
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		archive, err := waitForArchivedRecording(ctx, api.Recordings(), &Target{Id: 17}, &Recording{Id: 7, Name: "scorecard_test_rec"})
		Expect(archive).To(BeNil())
		Expect(err).To(MatchError(ContainSubstring("waiting for archive of recording scorecard_test_rec (ID 7)")))
		Expect(err).To(MatchError(context.DeadlineExceeded))
	})

	It("returns authorization errors without retrying", func() {
		var calls atomic.Int32
		api := testCryostatAPI(func(w http.ResponseWriter, _ *http.Request) {
			calls.Add(1)
			http.Error(w, "forbidden", http.StatusForbidden)
		})
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		archive, err := waitForArchivedRecording(ctx, api.Recordings(), &Target{Id: 17}, &Recording{Id: 7})
		Expect(archive).To(BeNil())
		Expect(err).To(MatchError(ContainSubstring("403")))
		Expect(calls.Load()).To(Equal(int32(1)))
	})

	DescribeTable("rejects unsuccessful GraphQL results instead of panicking", func(response, message string) {
		api := testCryostatAPI(func(w http.ResponseWriter, _ *http.Request) {
			_, err := w.Write([]byte(response))
			Expect(err).ToNot(HaveOccurred())
		})
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		archive, err := waitForArchivedRecording(ctx, api.Recordings(), &Target{Id: 17}, &Recording{Id: 7})
		Expect(archive).To(BeNil())
		Expect(err).To(MatchError(ContainSubstring(message)))
	},
		Entry("GraphQL error", `{"errors":[{"message":"archive query denied"}]}`, "archive query failed: archive query denied"),
		Entry("missing target", `{"data":{"targetNodes":[]}}`, "expected one target in archive query for target 17, got 0"),
	)
})
