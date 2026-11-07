# Build the manager binary
FROM docker.io/library/golang:1.19@sha256:3025bf670b8363ec9f1b4c4f27348e6d9b7fec607c47e401e40df816853e743a as builder

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY internal/main.go internal/main.go
COPY api/ api/
COPY internal/controllers/ internal/controllers/

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on go build -a -o manager internal/main.go

FROM registry.access.redhat.com/ubi8/ubi-minimal:latest
WORKDIR /
COPY --from=builder /workspace/manager .
USER 65532:65532

ENTRYPOINT ["/manager"]
