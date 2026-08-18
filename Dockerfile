# Build the manager binary
FROM registry.access.redhat.com/ubi9/go-toolset:9.8-1787080706@sha256:71e89a1a51ab32cc30634d89ee4dc8ea40ad9991057fa1eae3b1af32bc7db73f as builder
ARG TARGETOS
ARG TARGETARCH

# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY cmd/main.go cmd/main.go
COPY api/ api/
COPY internal/controller/ internal/controller/
COPY internal/console/ internal/console/
COPY internal/fips/ internal/fips/
COPY internal/webhook/ internal/webhook/

# Build
# the GOARCH has not a default value to allow the binary be built according to the host where the command
# was called. For example, if we call make oci-build in a local env which has the Apple Silicon M1 SO
# the docker BUILDPLATFORM arg will be linux/arm64 when for Apple x86 it will be linux/amd64. Therefore,
# by leaving it empty we can ensure that the container and binary shipped on it will have the same platform.
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} GO111MODULE=on go build -a -o manager cmd/main.go

FROM registry.access.redhat.com/ubi9/ubi-minimal:latest@sha256:8eb2830d0936237fc13a1f2f7e45aecf90d69043380ad167fad0343632937f41
WORKDIR /
COPY --from=builder /opt/app-root/src/manager .
USER 65532:65532

ENTRYPOINT ["/manager"]
