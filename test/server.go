package test

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
	jfrclient "github.com/rh-jmc-team/container-jfr-operator/pkg/client"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// WsMessage is a pair of an expected message for the server to receive along
// with a reply to send after receiving it
type WsMessage struct {
	ExpectedMsg *jfrclient.CommandMessage
	Reply       *jfrclient.ResponseMessage
}

// ContainerJFRServer is a test HTTP server used to simulate the Container JFR
// backend in unit tests
type ContainerJFRServer struct {
	impl *ghttp.Server
}

// NewServer creates a ContainerJFRServer for use by unit tests
func NewServer(client client.Client, messages []WsMessage) *ContainerJFRServer {
	s := ghttp.NewServer()
	clientURLResp := struct {
		ClientURL string `json:"clientUrl"`
	}{
		getClientURL(s),
	}
	s.RouteToHandler("GET", "/api/v1/clienturl", ghttp.RespondWithJSONEncoded(http.StatusOK, clientURLResp))
	s.RouteToHandler("GET", "/command", replayHandler(messages))

	// Update Service if present
	updateContainerJFRService(s, client)
	return &ContainerJFRServer{
		impl: s,
	}
}

// Close shuts down this test server
func (s *ContainerJFRServer) Close() {
	s.impl.Close()
}

func updateContainerJFRService(server *ghttp.Server, client client.Client) {
	// Look for the Container JFR service as returned by NewContainerJFRService
	svc := &corev1.Service{}
	ctx := context.Background()
	err := client.Get(ctx, types.NamespacedName{Name: "containerjfr", Namespace: "default"}, svc)
	if err == nil {
		// Fill in ClusterIP and Port with values from server URL
		serverURL, err := url.Parse(server.URL())
		gomega.Expect(err).ToNot(gomega.HaveOccurred())

		svc.Spec.ClusterIP = serverURL.Hostname()
		port, err := strconv.Atoi(serverURL.Port())
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
		svc.Spec.Ports[0].Port = int32(port)

		err = client.Update(ctx, svc)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
	}
}

func replayHandler(messages []WsMessage) http.HandlerFunc {
	return func(rw http.ResponseWriter, req *http.Request) {
		upgrader := websocket.Upgrader{}

		conn, err := upgrader.Upgrade(rw, req, nil)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
		defer conn.Close()

		for idx, msg := range messages {
			in := &jfrclient.CommandMessage{}
			err := conn.ReadJSON(in)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			// Check received message was what we expected
			msg.ExpectedMsg.ID = types.UID(strconv.Itoa(idx))
			gomega.Expect(in).To(gomega.Equal(msg.ExpectedMsg))

			// Write specified response
			msg.Reply.ID = types.UID(strconv.Itoa(idx))
			err = conn.WriteJSON(msg.Reply)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		}
	}
}

func getClientURL(server *ghttp.Server) string {
	return fmt.Sprintf("ws%s/command", strings.TrimPrefix(server.URL(), "http"))
}
