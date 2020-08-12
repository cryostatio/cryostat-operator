package test

import (
	"k8s.io/apimachinery/pkg/types"

	rhjmcv1alpha2 "github.com/rh-jmc-team/container-jfr-operator/pkg/apis/rhjmc/v1alpha2"
	jfrclient "github.com/rh-jmc-team/container-jfr-operator/pkg/client"
)

const tmpUID = types.UID("-1")

func NewDumpMessage() WsMessage {
	return WsMessage{
		ExpectedMsg: jfrclient.NewCommandMessage(
			"dump",
			"1.2.3.4:8001",
			tmpUID,
			"test-recording",
			"30",
			"jdk.socketRead:enabled=true,jdk.socketWrite:enabled=true"),
		Reply: &jfrclient.ResponseMessage{
			ID:          tmpUID,
			CommandName: "dump",
			Status:      jfrclient.ResponseStatusSuccess,
			Payload:     "",
		},
	}
}

func NewStartMessage() WsMessage {
	return WsMessage{
		ExpectedMsg: jfrclient.NewCommandMessage(
			"start",
			"1.2.3.4:8001",
			tmpUID,
			"test-recording",
			"jdk.socketRead:enabled=true,jdk.socketWrite:enabled=true"),
		Reply: &jfrclient.ResponseMessage{
			ID:          tmpUID,
			CommandName: "start",
			Status:      jfrclient.ResponseStatusSuccess,
			Payload:     "http://path/to/test-recording.jfr",
		},
	}
}

func NewStopMessage() WsMessage {
	return WsMessage{
		ExpectedMsg: jfrclient.NewCommandMessage(
			"stop",
			"1.2.3.4:8001",
			tmpUID,
			"test-recording"),
		Reply: &jfrclient.ResponseMessage{
			ID:          tmpUID,
			CommandName: "stop",
			Status:      jfrclient.ResponseStatusSuccess,
			Payload:     "",
		},
	}
}

func NewSaveMessage() WsMessage {
	return WsMessage{
		ExpectedMsg: jfrclient.NewCommandMessage(
			"save",
			"1.2.3.4:8001",
			tmpUID,
			"test-recording"),
		Reply: &jfrclient.ResponseMessage{
			ID:          tmpUID,
			CommandName: "save",
			Status:      jfrclient.ResponseStatusSuccess,
			Payload:     "saved-test-recording.jfr",
		},
	}
}

func NewListMessage(state string, duration int64) WsMessage {
	return WsMessage{
		ExpectedMsg: jfrclient.NewCommandMessage(
			"list",
			"1.2.3.4:8001",
			tmpUID),
		Reply: &jfrclient.ResponseMessage{
			ID:          tmpUID,
			CommandName: "list",
			Status:      jfrclient.ResponseStatusSuccess,
			Payload: []jfrclient.RecordingDescriptor{
				{
					Name:        "test-recording",
					State:       state,
					StartTime:   1597090030341,
					Duration:    duration,
					DownloadURL: "http://path/to/test-recording.jfr",
				},
			},
		},
	}
}

func NewListSavedMessage() WsMessage {
	return listSavedMessage([]jfrclient.SavedRecording{
		{
			Name:        "saved-test-recording.jfr",
			DownloadURL: "http://path/to/saved-test-recording.jfr",
			ReportURL:   "http://path/to/saved-test-recording.html",
		},
	})
}

func NewListSavedEmptyMessage() WsMessage {
	return listSavedMessage([]jfrclient.SavedRecording{})
}

func listSavedMessage(saved []jfrclient.SavedRecording) WsMessage {
	return WsMessage{
		ExpectedMsg: jfrclient.NewControlMessage(
			"list-saved",
			tmpUID),
		Reply: &jfrclient.ResponseMessage{
			ID:          tmpUID,
			CommandName: "list-saved",
			Status:      jfrclient.ResponseStatusSuccess,
			Payload:     saved,
		},
	}
}

func NewListEventTypesMessage() WsMessage {
	return WsMessage{
		ExpectedMsg: jfrclient.NewCommandMessage(
			"list-event-types",
			"1.2.3.4:8001",
			tmpUID),
		Reply: &jfrclient.ResponseMessage{
			ID:          tmpUID,
			CommandName: "list-event-types",
			Status:      jfrclient.ResponseStatusSuccess,
			Payload: []rhjmcv1alpha2.EventInfo{
				{
					TypeID:      "jdk.socketRead",
					Name:        "Socket Read",
					Description: "Reading data from a socket",
					Category:    []string{"Java Application"},
					Options: map[string]rhjmcv1alpha2.OptionDescriptor{
						"enabled": {
							Name:         "Enabled",
							Description:  "Record event",
							DefaultValue: "false",
						},
						"stackTrace": {
							Name:         "Stack Trace",
							Description:  "Record stack traces",
							DefaultValue: "false",
						},
						"threshold": {
							Name:         "Threshold",
							Description:  "Record event with duration above or equal to threshold",
							DefaultValue: "0ns[ns]",
						},
					},
				},
			},
		},
	}
}

func NewListEventTypesMessageFail() WsMessage {
	return WsMessage{
		ExpectedMsg: jfrclient.NewCommandMessage(
			"list-event-types",
			"1.2.3.4:8001",
			tmpUID),
		Reply: &jfrclient.ResponseMessage{
			ID:          tmpUID,
			CommandName: "list-event-types",
			Status:      jfrclient.ResponseStatusFailure,
			Payload:     "command failed",
		},
	}
}
