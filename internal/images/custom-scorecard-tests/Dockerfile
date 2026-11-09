# Copyright The Cryostat Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Build the manager binary
FROM registry.access.redhat.com/ubi9/go-toolset:9.8-1794208260@sha256:0e3894cea4fc077084245e8aa2befc8a7d91a4f60fbf850f0b5b555cbe62ee26 as builder
ARG TARGETOS
ARG TARGETARCH
ARG SCORECARD_SAMPLE_IMG=quay.io/redhat-java-monitoring/cryostat-operator-scorecard-app:latest
ARG SCORECARD_AGENT_IMG=quay.io/redhat-java-monitoring/cryostat-operator-scorecard-agent-app:latest

# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY api/ api/
COPY internal/controller/constants/ internal/controller/constants/
COPY internal/images/custom-scorecard-tests/main.go internal/images/custom-scorecard-tests/main.go
COPY internal/test/scorecard/ internal/test/scorecard/

# Build
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} GO111MODULE=on go build -a -o cryostat-scorecard-tests \
    -ldflags "-X github.com/cryostatio/cryostat-operator/internal/test/scorecard.SampleApplicationImage=${SCORECARD_SAMPLE_IMG} -X github.com/cryostatio/cryostat-operator/internal/test/scorecard.BundledAgentApplicationImage=${SCORECARD_AGENT_IMG}" \
    internal/images/custom-scorecard-tests/main.go

FROM registry.access.redhat.com/ubi9/ubi-minimal:latest@sha256:f26ef64172ea61f5ac5ba4be8c5c738bf8760b22b3e5148ce7b40f7308ab32db

ENV TEST=/usr/local/bin/cryostat-scorecard-tests \
    USER_UID=1001 \
    USER_NAME=test

COPY internal/images/custom-scorecard-tests/bin/user_setup /usr/local/bin/
COPY internal/images/custom-scorecard-tests/bin/entrypoint /usr/local/bin/
COPY --from=builder /opt/app-root/src/cryostat-scorecard-tests /usr/local/bin/
RUN  /usr/local/bin/user_setup

ENTRYPOINT ["/usr/local/bin/entrypoint"]

USER ${USER_UID}
