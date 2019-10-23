package flightrecorder

import (
	"encoding/json"
	"io"
	"io/ioutil"
	"net/url"
	"time"

	"github.com/gorilla/websocket"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var log = logf.Log.WithName("containerjfr_client")

type ClientConfig struct {
	ServerURL *url.URL
}

type ContainerJfrClient struct {
	config *ClientConfig
}

type CommandMessage struct {
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
}

func Create(config *ClientConfig) *ContainerJfrClient {
	return &ContainerJfrClient{config}
}

func (client *ContainerJfrClient) Connect() error {
	urlStr := client.config.ServerURL.String()
	conn, resp, err := websocket.DefaultDialer.Dial(urlStr, nil)
	if err != nil {
		log.Error(err, "failed to connect to command channel", "server", urlStr)
		if resp != nil {
			body, _ := ioutil.ReadAll(resp.Body)
			log.Info("response", "status", resp.Status, "body", string(body))
		}
		return err
	}
	defer conn.Close()

	done := make(chan struct{})
	gotRead := make(chan struct{}) // XXX

	go func() {
		defer close(done)
		for {
			_, msg, err := conn.ReadMessage() // TODO consider ReadJSON
			if err != nil {
				if err != io.EOF {
					log.Error(err, "could not read message")
				}
				return // TODO keep going?
			}
			log.Info("read message", "msg", string(msg))
			close(gotRead)
		}
	}()

	pingCmd := &CommandMessage{Command: "ping"}
	jsonCmd, err := json.Marshal(pingCmd)
	if err != nil {
		return err
	}
	log.Info("sending command", "json", string(jsonCmd))

	err = conn.WriteJSON(pingCmd)
	if err != nil {
		log.Error(err, "could not write message")
	}

	<-gotRead // XXX

	err = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	if err != nil {
		log.Error(err, "failed to close connection with server")
		return err
	} // TODO how to graceful shutdown

	select {
	case <-done:
		log.Info("closed connection with server")
		return nil
	case <-time.After(time.Second):
		log.Info("timed out waiting to close connection with server")
		return nil
	}
}
