package flightrecorder

import (
	"encoding/json"
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

type CommandMessage struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
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

Mesages can be out of order, maybe queue, and filter. Look at what container-jfr-web does

func newWebSocketConn(server *url.URL) (*websocket.Conn, error) {
	time.Sleep(time.Minute) // XXX
	urlStr := server.String()
	conn, resp, err := websocket.DefaultDialer.Dial(urlStr, nil)
	if err != nil {
		log.Error(err, "failed to connect to command channel", "server", urlStr)
		if resp != nil {
			body, _ := ioutil.ReadAll(resp.Body)
			log.Info("response", "status", resp.Status, "body", string(body))
		}
		return nil, err
	}
	/*conn.SetCloseHandler(func(code int, text string) error {
		// Attempt to reconnect
		log.Info("attempting to reconnect to server", "server", urlStr)
		conn.Close()
		err = client.newWebSocketConn(server)
		if err != nil {
			log.Error(err, "failed to reconnect to server")
			return err
		}
		return nil
	}) */
	return conn, nil
}

func (client *ContainerJfrClient) connect(host string, port int) error {
	target := fmt.Sprintf("%s:%d", host, port)
	connectCmd := &CommandMessage{Command: "connect", Args: []string{target}}
	jsonCmd, err := json.Marshal(connectCmd)
	if err != nil {
		return err
	}
	log.Info("sending command", "json", string(jsonCmd))

	err = client.conn.WriteJSON(connectCmd)
	if err != nil {
		log.Error(err, "could not write connect message")
		return err
	}

	_, msg, err := client.conn.ReadMessage()
	if err != nil {
		log.Error(err, "could not read connect message")
		return err
	}
	log.Info(string(msg))
	return nil
}

func (client *ContainerJfrClient) isConnected() error {
	isConnectedCmd := &CommandMessage{Command: "is-connected", Args: []string{}}
	jsonCmd, err := json.Marshal(isConnectedCmd)
	if err != nil {
		return err
	}
	log.Info("sending command", "json", string(jsonCmd))

	err = client.conn.WriteJSON(isConnectedCmd)
	if err != nil {
		log.Error(err, "could not write is-connected message")
		return err
	}

	_, msg, err := client.conn.ReadMessage()
	if err != nil {
		log.Error(err, "could not read is-connected message")
		return err
	}
	log.Info(string(msg))
	return nil
}

func (client *ContainerJfrClient) disconnect() error {
	disconnectCmd := &CommandMessage{Command: "disconnect", Args: []string{}}
	jsonCmd, err := json.Marshal(disconnectCmd)
	if err != nil {
		return err
	}
	log.Info("sending command", "json", string(jsonCmd))

	err = client.conn.WriteJSON(disconnectCmd)
	if err != nil {
		log.Error(err, "could not write disconnect message")
		return err
	}

	_, msg, err := client.conn.ReadMessage()
	if err != nil {
		log.Error(err, "could not read disconnect message")
		return err
	}
	log.Info(string(msg))
	return nil
}

func (client *ContainerJfrClient) ListRecordings(host string, port int) error {
	err := client.isConnected()
	if err != nil {
		return err
	}

	err = client.connect(host, port)
	if err != nil {
		return err
	}
	defer client.disconnect()

	listCmd := &CommandMessage{Command: "list", Args: []string{}}
	jsonCmd, err := json.Marshal(listCmd)
	if err != nil {
		return err
	}
	log.Info("sending command", "json", string(jsonCmd))

	err = client.conn.WriteJSON(listCmd)
	if err != nil {
		log.Error(err, "could not write list message")
		return err
	}

	_, msg, err := client.conn.ReadMessage()
	if err != nil {
		log.Error(err, "could not read list message")
		return err
	}
	log.Info(string(msg))
	return nil
}
