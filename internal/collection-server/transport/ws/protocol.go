package ws

import (
	"encoding/json"
)

const (
	OpSubscribe  = "subscribe"
	OpSubscribed = "subscribed"
	OpStatus     = "status"
	OpPing       = "ping"
	OpPong       = "pong"
	OpError      = "error"
)

type inboundFrame struct {
	Op           string `json:"op"`
	AssessmentID string `json:"assessment_id"`
	Kind         string `json:"kind"`
	TesteeID     string `json:"testee_id"`
}

type outboundFrame struct {
	Op           string `json:"op"`
	AssessmentID string `json:"assessment_id,omitempty"`
	Code         string `json:"code,omitempty"`
	Message      string `json:"message,omitempty"`
	Data         any    `json:"data,omitempty"`
}

func encodeFrame(frame outboundFrame) ([]byte, error) {
	return json.Marshal(frame)
}

func decodeFrame(payload []byte) (inboundFrame, error) {
	var frame inboundFrame
	if err := json.Unmarshal(payload, &frame); err != nil {
		return inboundFrame{}, err
	}
	return frame, nil
}
