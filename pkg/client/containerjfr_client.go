package client

import (
	"fmt"
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
	conn   *websocket.Conn
}

func Create(config *ClientConfig) (*ContainerJfrClient, error) {
	conn, err := newWebSocketConn(config.ServerURL)
	if err != nil {
		return nil, err
	}
	client := &ContainerJfrClient{config: config, conn: conn}
	return client, nil
}

func (client *ContainerJfrClient) Close() error {
	return client.conn.Close()
}

func newWebSocketConn(server *url.URL) (*websocket.Conn, error) {
	time.Sleep(time.Minute) // FIXME Use some kind of readiness probe to check when the server is ready
	urlStr := server.String()
	conn, resp, err := websocket.DefaultDialer.Dial(urlStr, nil)
	if err != nil {
		log.Error(err, "failed to connect to command channel", "server", urlStr)
		if resp != nil {
			body, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				return nil, err
			}
			log.Info("response", "status", resp.Status, "body", string(body))
		}
		return nil, err
	}
	return conn, nil
}

func (client *ContainerJfrClient) connect(host string, port int) error {
	target := fmt.Sprintf("%s:%d", host, port)
	connectCmd := NewCommandMessage("connect", target)
	var resp string
	err := client.syncMessage(connectCmd, &resp)
	if err != nil {
		return err
	}
	log.Info("got connect response", "resp", resp)
	return nil
}

func (client *ContainerJfrClient) isConnected() (bool, error) {
	isConnectedCmd := NewCommandMessage("is-connected")
	var resp string
	err := client.syncMessage(isConnectedCmd, &resp)
	if err != nil {
		return false, err
	}
	log.Info("got is-connected response", "resp", resp)
	isConnected := resp != "false"
	return isConnected, nil
}

func (client *ContainerJfrClient) disconnect() error {
	disconnectCmd := NewCommandMessage("disconnect")
	var resp string
	err := client.syncMessage(disconnectCmd, &resp)
	if err != nil {
		return err
	}
	log.Info("got disconnect response:", "resp", resp)
	return nil
}

// ListRecordings connects to a JVM addressed by the host and port and returns
// a list of its in-memory Flight Recordings
func (client *ContainerJfrClient) ListRecordings(host string, port int) error {
	connected, err := client.isConnected()
	if err != nil {
		return err
	} else if connected {
		log.Info("already connected, will disconnect first")
		err = client.disconnect()
		if err != nil {
			return err
		}
	}

	err = client.connect(host, port)
	if err != nil {
		return err
	}
	defer client.disconnect()

	listCmd := NewCommandMessage("list")
	recordings := []RecordingDescriptor{}
	err = client.syncMessage(listCmd, recordings)
	if err != nil {
		return err
	}
	log.Info("got list response", "resp", recordings)
	return nil
}

func (client *ContainerJfrClient) syncMessage(msg *CommandMessage, responsePayload interface{}) error {
	err := client.conn.WriteJSON(msg)
	if err != nil {
		log.Error(err, "could not write message", "message", msg)
		return err
	}
	log.Info("sent command", "json", msg)

	// By setting the output argument in the struct, the decoder knows what type
	// to unmarshall the payload into and also stores it for returing to the caller
	resp := &ResponseMessage{Payload: responsePayload}
	err = client.conn.ReadJSON(resp)
	if err != nil {
		log.Error(err, "could not read response", "message", msg)
		return err
	}
	log.Info("got response", "resp", resp)
	if resp.Status < 0 {
		// Parse exception/failure response and convert to error
		errMsg, ok := resp.Payload.(string)
		if !ok {
			errMsg = "unknown error response"
		}
		err = fmt.Errorf("server failed to execute \"%s\": %s (code %d)", resp.CommandName, errMsg, resp.Status)
		log.Error(err, "command failed", "request", msg)
		return err
	}
	return err
}
