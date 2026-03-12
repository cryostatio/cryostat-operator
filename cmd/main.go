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

package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	"k8s.io/client-go/discovery"
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	certv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	configv1 "github.com/openshift/api/config/v1"
	consolev1 "github.com/openshift/api/console/v1"
	openshiftoperatorv1 "github.com/openshift/api/operator/v1"
	routev1 "github.com/openshift/api/route/v1"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/certwatcher"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	ctrlwebhook "sigs.k8s.io/controller-runtime/pkg/webhook"

	operatorv1beta1 "github.com/cryostatio/cryostat-operator/api/v1beta1"
	operatorv1beta2 "github.com/cryostatio/cryostat-operator/api/v1beta2"
	"github.com/cryostatio/cryostat-operator/internal/console"
	"github.com/cryostatio/cryostat-operator/internal/controller"
	"github.com/cryostatio/cryostat-operator/internal/controller/common"
	"github.com/cryostatio/cryostat-operator/internal/controller/constants"
	"github.com/cryostatio/cryostat-operator/internal/fips"
	"github.com/cryostatio/cryostat-operator/internal/webhook/agent"
	webhook "github.com/cryostatio/cryostat-operator/internal/webhook/v1beta2"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(operatorv1beta1.AddToScheme(scheme))

	// Register third-party types
	utilruntime.Must(routev1.AddToScheme(scheme))
	utilruntime.Must(certv1.AddToScheme(scheme))
	utilruntime.Must(consolev1.AddToScheme(scheme))
	utilruntime.Must(configv1.AddToScheme(scheme))
	utilruntime.Must(openshiftoperatorv1.AddToScheme(scheme))

	utilruntime.Must(operatorv1beta2.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

// nolint:gocyclo
func main() {
	var metricsAddr string
	var metricsCertPath, metricsCertName, metricsCertKey string
	var webhookCertPath, webhookCertName, webhookCertKey string
	var enableLeaderElection bool
	var probeAddr string
	var secureMetrics bool
	var enableHTTP2 bool
	var forceOpenShift bool
	var consolePlugin bool
	var tlsOpts []func(*tls.Config)

	flag.StringVar(&metricsAddr, "metrics-bind-address", "0", "The address the metrics endpoint binds to. "+
		"Use :8443 for HTTPS or :8080 for HTTP, or leave as 0 to disable the metrics service.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&secureMetrics, "metrics-secure", true,
		"If set, the metrics endpoint is served securely via HTTPS. Use --metrics-secure=false to use HTTP instead.")
	flag.StringVar(&webhookCertPath, "webhook-cert-path", "", "The directory that contains the webhook certificate.")
	flag.StringVar(&webhookCertName, "webhook-cert-name", "tls.crt", "The name of the webhook certificate file.")
	flag.StringVar(&webhookCertKey, "webhook-cert-key", "tls.key", "The name of the webhook key file.")
	flag.StringVar(&metricsCertPath, "metrics-cert-path", "",
		"The directory that contains the metrics server certificate.")
	flag.StringVar(&metricsCertName, "metrics-cert-name", "tls.crt", "The name of the metrics server certificate file.")
	flag.StringVar(&metricsCertKey, "metrics-cert-key", "tls.key", "The name of the metrics server key file.")
	flag.BoolVar(&enableHTTP2, "enable-http2", false,
		"If set, HTTP/2 will be enabled for the metrics and webhook servers")
	flag.BoolVar(&forceOpenShift, "force-openshift", false, "Force the controller to consider current platform as OpenShift")
	flag.BoolVar(&consolePlugin, "openshift-console-plugin", false, "Whether the operator should install the OpenShift Console Plugin")

	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// if the enable-http2 flag is false (the default), http/2 should be disabled
	// due to its vulnerabilities. More specifically, disabling http/2 will
	// prevent from being vulnerable to the HTTP/2 Stream Cancellation and
	// Rapid Reset CVEs. For more information see:
	// - https://github.com/advisories/GHSA-qppj-fm5r-hxr3
	// - https://github.com/advisories/GHSA-4374-p667-p6c8
	disableHTTP2 := func(c *tls.Config) {
		setupLog.Info("disabling http/2")
		c.NextProtos = []string{"http/1.1"}
	}

	if !enableHTTP2 {
		tlsOpts = append(tlsOpts, disableHTTP2)
	}

	// Create watchers for metrics and webhooks certificates
	var metricsCertWatcher, webhookCertWatcher *certwatcher.CertWatcher

	// Initial webhook TLS options
	webhookTLSOpts := tlsOpts

	if len(webhookCertPath) > 0 {
		setupLog.Info("Initializing webhook certificate watcher using provided certificates",
			"webhook-cert-path", webhookCertPath, "webhook-cert-name", webhookCertName, "webhook-cert-key", webhookCertKey)

		var err error
		webhookCertWatcher, err = certwatcher.New(
			filepath.Join(webhookCertPath, webhookCertName),
			filepath.Join(webhookCertPath, webhookCertKey),
		)
		if err != nil {
			setupLog.Error(err, "Failed to initialize webhook certificate watcher")
			os.Exit(1)
		}

		webhookTLSOpts = append(webhookTLSOpts, func(config *tls.Config) {
			config.GetCertificate = webhookCertWatcher.GetCertificate
		})
	}

	webhookServer := ctrlwebhook.NewServer(ctrlwebhook.Options{
		TLSOpts: webhookTLSOpts,
	})

	// Metrics endpoint is enabled in 'config/default/kustomization.yaml'. The Metrics options configure the server.
	// More info:
	// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.21.0/pkg/metrics/server
	// - https://book.kubebuilder.io/reference/metrics.html
	metricsServerOptions := metricsserver.Options{
		BindAddress:   metricsAddr,
		SecureServing: secureMetrics,
		TLSOpts:       tlsOpts,
	}

	if secureMetrics {
		// FilterProvider is used to protect the metrics endpoint with authn/authz.
		// These configurations ensure that only authorized users and service accounts
		// can access the metrics endpoint. The RBAC are configured in 'config/rbac/kustomization.yaml'. More info:
		// https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.21.0/pkg/metrics/filters#WithAuthenticationAndAuthorization
		metricsServerOptions.FilterProvider = filters.WithAuthenticationAndAuthorization
	}

	// If the certificate is not specified, controller-runtime will automatically
	// generate self-signed certificates for the metrics server. While convenient for development and testing,
	// this setup is not recommended for production.
	if len(metricsCertPath) > 0 {
		setupLog.Info("Initializing metrics certificate watcher using provided certificates",
			"metrics-cert-path", metricsCertPath, "metrics-cert-name", metricsCertName, "metrics-cert-key", metricsCertKey)

		var err error
		metricsCertWatcher, err = certwatcher.New(
			filepath.Join(metricsCertPath, metricsCertName),
			filepath.Join(metricsCertPath, metricsCertKey),
		)
		if err != nil {
			setupLog.Error(err, "to initialize metrics certificate watcher", "error", err)
			os.Exit(1)
		}

		metricsServerOptions.TLSOpts = append(metricsServerOptions.TLSOpts, func(config *tls.Config) {
			config.GetCertificate = metricsCertWatcher.GetCertificate
		})
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsServerOptions,
		WebhookServer:          webhookServer,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "d696d7ab.redhat.com",
		// LeaderElectionReleaseOnCancel defines if the leader should step down voluntarily
		// when the Manager ends. This requires the binary to immediately end when the
		// Manager is stopped, otherwise, this setting is unsafe. Setting this significantly
		// speeds up voluntary leader transitions as the new leader don't have to wait
		// LeaseDuration time first.
		//
		// In the default scaffold provided, the program ends immediately after
		// the manager stops, so would be fine to enable this option. However,
		// if you are doing or is intended to do any operation such as perform cleanups
		// after the manager stops then its usage might be unsafe.
		// LeaderElectionReleaseOnCancel: true,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// To look for particular resource types on the API server
	dc, err := discovery.NewDiscoveryClientForConfig(mgr.GetConfig())
	if err != nil {
		setupLog.Error(err, "failed to create discovery client")
		os.Exit(1)
	}

	openShift, err := isOpenShift(dc, forceOpenShift)
	if err != nil {
		setupLog.Error(err, "could not determine whether manager is running on OpenShift")
		os.Exit(1)
	}
	environment := "Kubernetes"
	if openShift {
		environment = "OpenShift"
	}
	setupLog.Info(fmt.Sprintf("detected %s environment", environment))

	certManager, err := isCertManagerInstalled(dc)
	if err != nil {
		setupLog.Error(err, "could not determine whether cert-manager is installed")
		os.Exit(1)
	}
	if certManager {
		setupLog.Info("found cert-manager installation")
	} else {
		setupLog.Info("did not find cert-manager installation")
	}

	// If this is an OpenShift cluster, check if it's running in FIPS mode
	fipsEnabled := false
	if openShift {
		result, err := fips.IsFIPS(mgr.GetAPIReader())
		if err != nil {
			setupLog.Error(err, "could not determine whether FIPS is enabled, assuming disabled")
		} else {
			fipsEnabled = result
			if fipsEnabled {
				setupLog.Info("detected FIPS mode for this cluster")
			}
		}
	}

	// Optionally install OpenShift Console Plugin
	if consolePlugin {
		// Look up operator namespace
		namespace := os.Getenv("OPERATOR_NAMESPACE")
		if len(namespace) == 0 {
			setupLog.Error(err, "could not determine operator's namespace")
			os.Exit(1)
		}
		installer := &console.PluginInstaller{
			Client:    mgr.GetClient(),
			Namespace: namespace,
			Scheme:    mgr.GetScheme(),
			Log:       setupLog,
		}
		err := installer.SetupWithManager(mgr)
		if err != nil {
			setupLog.Error(err, "failed to install OpenShift Console Plugin")
			os.Exit(1)
		}
	}

	// Optionally enable Insights integration. Will only be enabled if INSIGHTS_ENABLED is true
	var insightsURL *url.URL
	if openShift {
		insightsEnabledEnv := os.Getenv("INSIGHTS_ENABLED")
		if strings.ToLower(insightsEnabledEnv) == "true" {
			insightsURLEnv := os.Getenv("INSIGHTS_URL")
			if len(insightsURLEnv) > 0 {
				insightsURL, err = url.Parse(insightsURLEnv)
				if err != nil {
					setupLog.Error(err, "INSIGHTS_URL is invalid")
					os.Exit(1)
				}
			}
		}
	}

	config := newReconcilerConfig(mgr, "Cryostat", "cryostat-controller", openShift, certManager,
		insightsURL)
	cryostatController, err := controller.NewCryostatReconciler(config)
	if err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Cryostat")
		os.Exit(1)
	}
	if err = cryostatController.SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to add controller to manager", "controller", "Cryostat")
		os.Exit(1)
	}
	// nolint:goconst
	if os.Getenv("ENABLE_WEBHOOKS") != "false" {
		if err = webhook.SetupWebhookWithManager(mgr, &operatorv1beta2.Cryostat{}); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "Cryostat")
			os.Exit(1)
		}
		agentWebhook := agent.NewAgentWebhook(&agent.AgentWebhookConfig{
			FIPSEnabled: fipsEnabled,
		})
		if err = agentWebhook.SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "Pod")
			os.Exit(1)
		}
	}
	// +kubebuilder:scaffold:builder

	if metricsCertWatcher != nil {
		setupLog.Info("Adding metrics certificate watcher to manager")
		if err := mgr.Add(metricsCertWatcher); err != nil {
			setupLog.Error(err, "unable to add metrics certificate watcher to manager")
			os.Exit(1)
		}
	}

	if webhookCertWatcher != nil {
		setupLog.Info("Adding webhook certificate watcher to manager")
		if err := mgr.Add(webhookCertWatcher); err != nil {
			setupLog.Error(err, "unable to add webhook certificate watcher to manager")
			os.Exit(1)
		}
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager", "version", constants.OperatorVersion)
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

func isOpenShift(client discovery.DiscoveryInterface, forceOpenShift bool) (bool, error) {
	if forceOpenShift {
		return true, nil
	}
	return discovery.IsResourceEnabled(client, routev1.GroupVersion.WithResource("routes"))
}

func isCertManagerInstalled(client discovery.DiscoveryInterface) (bool, error) {
	return discovery.IsResourceEnabled(client, certv1.SchemeGroupVersion.WithResource("issuers"))
}

func newReconcilerConfig(mgr ctrl.Manager, logName string, eventRecorderName string, openShift bool,
	certManager bool, insightsURL *url.URL) *controller.ReconcilerConfig {
	return &controller.ReconcilerConfig{
		Client:                 mgr.GetClient(),
		Log:                    ctrl.Log.WithName("controller").WithName(logName),
		Scheme:                 mgr.GetScheme(),
		IsOpenShift:            openShift,
		IsCertManagerInstalled: certManager,
		EventRecorder:          mgr.GetEventRecorderFor(eventRecorderName),
		RESTMapper:             mgr.GetRESTMapper(),
		InsightsProxy:          insightsURL,
		NewControllerBuilder:   common.NewControllerBuilder,
		ReconcilerTLS: common.NewReconcilerTLS(&common.ReconcilerTLSConfig{
			Client: mgr.GetClient(),
		}),
	}
}
