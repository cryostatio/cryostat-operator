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
FROM registry.access.redhat.com/ubi9/go-toolset:9.8-1785360201@sha256:272a3dd990bba320c2e246119a0019d10d627d0104938d5db77ba5ab3ae3a51d as builder
ARG TARGETOS
ARG TARGETARCH

# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY api/ api/
COPY internal/images/custom-scorecard-tests/main.go internal/images/custom-scorecard-tests/main.go
COPY internal/test/scorecard/ internal/test/scorecard/

# Build
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} GO111MODULE=on go build -a -o cryostat-scorecard-tests \
    internal/images/custom-scorecard-tests/main.go

FROM registry.access.redhat.com/ubi9/ubi-minimal:latest@sha256:17fd831ced9434de0a984d60b3fbe61008308261ba98bbc348d6fbdef05fa7c0

ENV TEST=/usr/local/bin/cryostat-scorecard-tests \
    USER_UID=1001 \
    USER_NAME=test

COPY internal/images/custom-scorecard-tests/bin/user_setup /usr/local/bin/
COPY internal/images/custom-scorecard-tests/bin/entrypoint /usr/local/bin/
COPY --from=builder /opt/app-root/src/cryostat-scorecard-tests /usr/local/bin/
RUN  /usr/local/bin/user_setup

ENTRYPOINT ["/usr/local/bin/entrypoint"]

USER ${USER_UID}
