package client

import (
	"encoding/json"
)

type CommandMessage struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

func NewCommandMessage(command string, args ...string) *CommandMessage {
	return &CommandMessage{Command: command, Args: args}
}

type ResponseMessage struct {
	CommandName string      `json:"commandName"`
	Status      int         `json:"status"`
	Payload     interface{} `json:"payload"`
}

type RecordingState string

const (
	RecordingStateCreated  RecordingState = "CREATED"
	RecordingStateRunning  RecordingState = "RUNNING"
	RecordingStateStopping RecordingState = "STOPPING"
	RecordingStateStopped  RecordingState = "STOPPED"
)

type RecordingDescriptor struct {
	ID          int64          `json:"id"`
	Name        string         `json:"name"`
	State       RecordingState `json:"state"`
	StartTime   int64          `json:"startTime"`
	Duration    int64          `json:"duration"`
	Continuous  bool           `json:"continuous"`
	ToDisk      bool           `json:"toDisk"`
	MaxSize     int64          `json:"maxSize"`
	MaxAge      int64          `json:"maxAge"`
	DownloadURL string         `json:"downloadUrl"`
	ReportURL   string         `json:"reportUrl"`
}

// UnmarshalJSON overrides standard JSON parsing to handle error payloads
func (msg *ResponseMessage) UnmarshalJSON(data []byte) error {
	// Unmarshall only status at first to determine if we need to
	// parse an error string
	peekStatus := struct {
		Status int `json:"status"`
	}{}
	err := json.Unmarshal(data, &peekStatus)
	if err != nil {
		return err
	}
	if peekStatus.Status < 0 {
		// Expect a string payload for non-zero status
		msg.Payload = ""
	}
	// Use an alias to prevent infinite recursion on this method
	type responseMessageAlias ResponseMessage
	err = json.Unmarshal(data, (*responseMessageAlias)(msg))
	return err
}
