package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strings"

	rhjmcv1alpha1 "github.com/rh-jmc-team/container-jfr-operator/pkg/apis/rhjmc/v1alpha1"
	jfrclient "github.com/rh-jmc-team/container-jfr-operator/pkg/client"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var log = logf.Log.WithName("common_reconciler")

type CommonReconciler struct {
	// This client, initialized using mgr.Client(), is a split client
	// that reads objects from the cache and writes to the apiserver
	Client    client.Client
	JfrClient *jfrclient.ContainerJfrClient
}

// FindContainerJFR retrieves a ContainerJFR instance within a given namespace
func (r *CommonReconciler) FindContainerJFR(ctx context.Context, namespace string) (*rhjmcv1alpha1.ContainerJFR, error) {
	// TODO Consider how to find ContainerJFR object if this operator becomes cluster-scoped
	// Look up the ContainerJFR object for this operator, which will help us find its services
	cjfrList := &rhjmcv1alpha1.ContainerJFRList{}
	err := r.Client.List(ctx, cjfrList)
	if err != nil {
		return nil, err
	}
	if len(cjfrList.Items) == 0 {
		return nil, errors.New("No ContainerJFR objects found")
	} else if len(cjfrList.Items) > 1 {
		// Does not seem like a proper use-case
		log.Info("More than one ContainerJFR object found in namespace, using only the first one listed",
			"namespace", namespace)
	}
	return &cjfrList.Items[0], nil
}

func (r *CommonReconciler) ConnectToContainerJFR(ctx context.Context, namespace string,
	svcName string) (*jfrclient.ContainerJfrClient, error) {
	// Query the "clienturl" endpoint of Container JFR for the command URL
	clientURL, err := r.getClientURL(ctx, namespace, svcName)
	if err != nil {
		return nil, err
	}
	tok, err := ioutil.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/token")
	if err != nil {
		return nil, err
	}
	strTok := string(tok)
	config := &jfrclient.Config{ServerURL: clientURL, AccessToken: &strTok, TLSVerify: !strings.EqualFold(os.Getenv("TLS_VERIFY"), "false")}
	jfrClient, err := jfrclient.Create(config)
	if err != nil {
		return nil, err
	}
	return jfrClient, nil
}

func (r *CommonReconciler) ConnectToService(targetSvc *corev1.Service, jmxPort int32) error {
	// Have Container JFR connect to the target JVM
	clusterIP, err := getClusterIP(targetSvc)
	if err != nil {
		return err
	}
	err = r.JfrClient.Connect(*clusterIP, jmxPort)
	if err != nil {
		log.Error(err, "failed to connect to target JVM")
		r.CloseClient()
		return err
	}
	log.Info("Container JFR connected to service", "service", targetSvc.Name, "namespace", targetSvc.Namespace,
		"host", *clusterIP, "port", jmxPort)
	return nil
}

func (r *CommonReconciler) DisconnectClient() {
	if r.JfrClient != nil {
		err := r.JfrClient.Disconnect()
		if err != nil {
			log.Error(err, "failed to disconnect from target JVM")
			r.CloseClient()
		}
	}
}

func (r *CommonReconciler) CloseClient() {
	r.JfrClient.Close()
	r.JfrClient = nil
}

func (r *CommonReconciler) getClientURL(ctx context.Context, namespace string, svcName string) (*url.URL, error) {
	// Look up Container JFR service, and query "clienturl" endpoint
	cjfrSvc := &corev1.Service{}
	err := r.Client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: svcName}, cjfrSvc)
	if err != nil {
		return nil, err
	}
	clusterIP, err := getClusterIP(cjfrSvc)
	if err != nil {
		return nil, err
	}
	webServerPort, err := getWebServerPort(cjfrSvc)
	if err != nil {
		return nil, err
	}
	host := fmt.Sprintf("http://%s:%d/clienturl", *clusterIP, webServerPort)
	resp, err := http.Get(host)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Find "clientUrl" JSON property in repsonse
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	clientURLHolder := struct {
		ClientURL string `json:"clientUrl"`
	}{}
	err = json.Unmarshal(body, &clientURLHolder)
	if err != nil {
		return nil, err
	}
	return url.Parse(clientURLHolder.ClientURL)
}

func getClusterIP(svc *corev1.Service) (*string, error) {
	clusterIP := svc.Spec.ClusterIP
	if clusterIP == "" || clusterIP == corev1.ClusterIPNone {
		return nil, fmt.Errorf("ClusterIP unavailable for %s", svc.Name)
	}
	return &clusterIP, nil
}

func getWebServerPort(svc *corev1.Service) (int32, error) {
	for _, port := range svc.Spec.Ports {
		if port.Name == "export" {
			return port.Port, nil
		}
	}
	return 0, errors.New("ContainerJFR service had no port named \"export\"")
}
