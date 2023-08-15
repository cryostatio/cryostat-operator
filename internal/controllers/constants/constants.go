package constants

import (
	certMeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
)

const (
	CryostatHTTPContainerPort int32  = 8181
	CryostatJMXContainerPort  int32  = 9091
	GrafanaContainerPort      int32  = 3000
	DatasourceContainerPort   int32  = 8080
	ReportsContainerPort      int32  = 10000
	LoopbackAddress           string = "127.0.0.1"
	OperatorNamePrefix        string = "cryostat-operator-"
	HttpPortName              string = "http"
	// CAKey is the key for a CA certificate within a TLS secret
	CAKey = certMeta.TLSCAKey
	// Hostname alias for loopback address, to be used for health checks
	HealthCheckHostname = "cryostat-health.local"
)
