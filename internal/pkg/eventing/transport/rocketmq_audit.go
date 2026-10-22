package transport

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/FangcunMount/qs-server/internal/pkg/eventing/deliveryevidence"
	rmtransport "github.com/FangcunMount/reliable-messaging/transport"
)

// TransportEvidence preserves the original wire independently of the decoded
// business payload used by the existing, manually governed replay path.
type TransportEvidence struct {
	ApplicationID, PhysicalTopic string
	Wire, Metadata               []byte
}

func rocketMQDeadLetterRecord(m rmtransport.Received, cause string) (DeadLetterRecord, error) {
	if m.Provider != "rocketmq" || m.Topic == "" || m.Channel == "" || m.PhysicalTopic == "" || m.TransportID == "" || len(m.Wire) == 0 {
		return DeadLetterRecord{}, errors.New("RocketMQ failure source evidence required")
	}
	metadata, err := json.Marshal(m.Metadata)
	if err != nil {
		return DeadLetterRecord{}, err
	}
	id, payload := m.ID, m.Payload
	if id == "" {
		// This is an audit key. The absent business identity is kept separately.
		id = fmt.Sprintf("invalid-wire:%x", sha256.Sum256(m.Wire))
		// The legacy replay column is UTF-8 text; arbitrary invalid wire belongs
		// in the binary evidence table, with an explicit non-business wrapper.
		payload, err = json.Marshal(struct {
			Type string `json:"type"`
			Wire []byte `json:"original_wire_base64"`
		}{"invalid-transport-wire.v1", m.Wire})
		if err != nil {
			return DeadLetterRecord{}, err
		}
	}
	r := deadLetterRecord("rocketmq", m.Topic, m.Channel, max(1, int(m.Attempts)), id, m.TransportID, payload, cause)
	r.Evidence = &TransportEvidence{ApplicationID: m.ID, PhysicalTopic: m.PhysicalTopic, Wire: append([]byte(nil), m.Wire...), Metadata: metadata}
	return r, nil
}

func SDKRocketMQFailureHandler(recorder DeadLetterRecorder) func(context.Context, rmtransport.Failure) error {
	return func(ctx context.Context, f rmtransport.Failure) error {
		if recorder == nil || f.Provider != "rocketmq" || f.Cause == "" {
			return errors.New("RocketMQ durable failure recorder required")
		}
		r, err := rocketMQDeadLetterRecord(f.Message, f.Cause)
		if err != nil {
			return err
		}
		return recorder.RecordDeadLetter(ctx, r)
	}
}

func recordTransportEvidence(ctx context.Context, tx *sql.Tx, r DeadLetterRecord) error {
	if r.Evidence == nil {
		return errors.New("RocketMQ original transport evidence required before ACK")
	}
	e := r.Evidence
	return deliveryevidence.Persist(ctx, tx, deliveryevidence.Source{Provider: r.Provider, Topic: r.Topic, Channel: r.Channel, TransportMessageID: r.TransportMessageID, ApplicationID: e.ApplicationID, PhysicalTopic: e.PhysicalTopic, Wire: e.Wire, Metadata: e.Metadata})
}
