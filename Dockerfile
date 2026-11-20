# Build the manager binary
FROM docker.io/library/golang:1.17@sha256:87262e4a4c7db56158a80a18fefdc4fee5accc41b59cde821e691d05541bbb18 as builder

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

FROM registry.access.redhat.com/ubi8/ubi-minimal:latest@sha256:0990c03b74757479e30f381206efa1387ea2d5df037ede48c494f1acb041cf69
WORKDIR /
COPY --from=builder /workspace/manager .
USER 65532:65532

ENTRYPOINT ["/manager"]
