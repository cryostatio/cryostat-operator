package client

import (
	"encoding/json"
	"errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"
)

// CommandMessage represents the body of a command request to be sent
// to Container JFR
type CommandMessage struct {
	// ID used to uniquely identify a response to this message
	ID types.UID `json:"id"`
	// The name of the command, must be recognized by Container JFR
	Command string `json:"command"`
	// Any arguments that Container JFR accepts for the named command
	Args []string `json:"args"`
}

// NewCommandMessage provides a conventient shorthand for constructing
// new CommandMessages
func NewCommandMessage(command string, args ...string) *CommandMessage {
	return &CommandMessage{ID: uuid.NewUUID(), Command: command, Args: args}
}

// ResponseStatus is a response code used by Container JFR to indiciate
// the outcome of executed commands
type ResponseStatus int

const (
	// ResponseStatusSuccess indicates that the command succeeded
	ResponseStatusSuccess ResponseStatus = 0
	// ResponseStatusFailure indicates that the command failed due to
	// invalid state/arguments, such as a recording of a given name
	// already existing
	ResponseStatusFailure ResponseStatus = -1
	// ResponseStatusException indicates that an unexpected error
	// occurred while executing the command
	ResponseStatusException ResponseStatus = -2
)

// ResponseMessage corresponds to the response from Container JFR to
// a command message previously sent
type ResponseMessage struct {
	// Identifier of the command message that triggered this response
	ID types.UID `json:"id"`
	// The name of the command that this message is responding to
	CommandName string `json:"commandName"`
	// The response code showing whether this command succeeded
	Status ResponseStatus `json:"status"`
	// The data expected in response to the issued command. The type
	// of payload will differ depending on the command and whether
	// the command was successful
	Payload interface{} `json:"payload"`
}

// RecordingDescriptor contains various metadata for a particular
// flight recording retrieved from the JVM
type RecordingDescriptor struct {
	// An identifier used by the JVM to uniquely identify a recording
	ID int64 `json:"id"`
	// Name of the recording specified during creation
	Name string `json:"name"`
	// State of the recording within its lifecycle
	State string `json:"state"`
	// Time when the recording first started, in milliseconds since Unix epoch
	StartTime int64 `json:"startTime"`
	// How long the recording was configured to run for, in milliseconds
	Duration int64 `json:"duration"`
	// Whether the recording was configured to record indefinitely
	Continuous bool `json:"continuous"`
	// Whether this recording was dumped to disk in the host containing the JVM
	ToDisk bool `json:"toDisk"`
	// The maximum configured size of the recording file
	MaxSize int64 `json:"maxSize"`
	// The maximum configured age of recorded events
	MaxAge int64 `json:"maxAge"`
	// URL to download the raw flight recording file
	DownloadURL string `json:"downloadUrl"`
	// URL to the automated analysis report for this recording
	ReportURL string `json:"reportUrl"`
}

// SavedRecording represents a recording file that has been archived in
// persistent storage by Container JFR
type SavedRecording struct {
	Name        string `json:"name"`
	DownloadURL string `json:"downloadUrl"`
	ReportURL   string `json:"reportUrl"`
}

// ErrWrongID is returned when a JSON response has an unexpected ID
var ErrWrongID error = errors.New("Response in reply to different command")

// UnmarshalJSON overrides standard JSON parsing to handle error payloads
// and filter out messages intended for other clients.
// The receiver's ID should be populated with the command message's ID
// and the Payload should be set to the expected type's zero value.
func (msg *ResponseMessage) UnmarshalJSON(data []byte) error {
	// Unmarshall ID to check if we are the intended recipient, and
	// also status to determine if we need to parse an error string
	peek := struct {
		ID     types.UID `json:"id"`
		Status int       `json:"status"`
	}{}
	err := json.Unmarshal(data, &peek)
	if err != nil {
		return err
	}
	// By checking the ID here, we can abort early and not attempt
	// to parse a response which could contain a different kind of
	// payload
	if msg.ID != peek.ID {
		debugLog.Info("Skipping response with unexpected ID", "expected", msg.ID,
			"actual", peek.ID)
		return ErrWrongID
	}
	if peek.Status < 0 {
		// Expect a string payload for non-zero status
		msg.Payload = ""
	}
	// Use an alias to prevent infinite recursion on this method
	type responseMessageAlias ResponseMessage
	err = json.Unmarshal(data, (*responseMessageAlias)(msg))
	return err
}
