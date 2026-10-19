package transport

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

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
	e := r.Evidence
	if e == nil || e.PhysicalTopic == "" || len(e.Wire) == 0 || !json.Valid(e.Metadata) {
		return errors.New("RocketMQ original transport evidence required before ACK")
	}
	id := sha256.Sum256([]byte(r.Provider + "\x00" + r.Topic + "\x00" + r.Channel + "\x00" + r.TransportMessageID))
	_, err := tx.ExecContext(ctx, `INSERT INTO event_delivery_transport_evidence
(identity_hash,provider,topic_name,channel_name,transport_message_id,application_id,physical_topic,original_wire,metadata_bytes)
VALUES(?,?,?,?,?,?,?,?,?) ON DUPLICATE KEY UPDATE identity_hash=identity_hash`,
		id[:], r.Provider, r.Topic, r.Channel, r.TransportMessageID, e.ApplicationID, e.PhysicalTopic, e.Wire, e.Metadata)
	if err != nil {
		return err
	}
	var provider, topic, channel, transportID, application, physical string
	var wire, metadata []byte
	err = tx.QueryRowContext(ctx, `SELECT provider,topic_name,channel_name,transport_message_id,application_id,physical_topic,original_wire,metadata_bytes FROM event_delivery_transport_evidence WHERE identity_hash=? FOR UPDATE`, id[:]).Scan(&provider, &topic, &channel, &transportID, &application, &physical, &wire, &metadata)
	if err != nil {
		return err
	}
	if provider != r.Provider || topic != r.Topic || channel != r.Channel || transportID != r.TransportMessageID || application != e.ApplicationID || physical != e.PhysicalTopic || !bytes.Equal(wire, e.Wire) || !bytes.Equal(metadata, e.Metadata) {
		return errors.New("RocketMQ transport identity reused with different immutable evidence")
	}
	return nil
}
