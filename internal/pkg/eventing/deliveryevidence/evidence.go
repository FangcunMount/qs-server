// Package deliveryevidence preserves immutable physical-delivery evidence in a borrowed original SQL transaction.
package deliveryevidence

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	rmtransport "github.com/FangcunMount/reliable-messaging/transport"
	"github.com/FangcunMount/reliable-messaging/wire/legacy"
)

type Source struct {
	Provider, Topic, Channel, TransportMessageID string
	ApplicationID, PhysicalTopic                 string
	Wire, Metadata                               []byte
}

// RocketMQDeliveryIdentity selects immutable evidence in the fixed host store.
// Callers still verify and persist the complete original wire before Ack.
func RocketMQDeliveryIdentity(message rmtransport.Received) ([32]byte, error) {
	if message.Provider != "rocketmq" || message.Topic == "" || message.Channel == "" || message.TransportID == "" {
		return [32]byte{}, errors.New("trusted RocketMQ delivery identity required")
	}
	return Identity(message.Provider, message.Topic, message.Channel, message.TransportID), nil
}

// PreserveRocketMQDelivery borrows the caller's original SQL transaction. A
// deferred delivery and its exact original wire must commit together before Ack.
// It opens no connection and grants no publication or business retry authority.
func PreserveRocketMQDelivery(ctx context.Context, tx *sql.Tx, message rmtransport.Received) ([32]byte, error) {
	if tx == nil || message.ID == "" || len(message.Wire) > 262144 || len(message.Payload) > 262144 {
		return [32]byte{}, errors.New("original transaction and application identity required")
	}
	if message.Provider != "rocketmq" || message.Topic == "" || message.Channel == "" || message.TransportID == "" || message.PhysicalTopic == "" || len(message.Wire) == 0 {
		return [32]byte{}, errors.New("RocketMQ original transport source required")
	}
	encodedMetadata, err := json.Marshal(message.Metadata)
	if err != nil {
		return [32]byte{}, err
	}
	envelope, recognized, err := legacy.Decode(message.Wire)
	if err != nil || !recognized || envelope.UUID != message.ID || !bytes.Equal(envelope.Payload, message.Payload) {
		return [32]byte{}, errors.New("deferred payload differs from original RocketMQ wire")
	}
	metadata, err := json.Marshal(envelope.Metadata)
	if err != nil || len(metadata) > 65535 || !bytes.Equal(metadata, encodedMetadata) {
		return [32]byte{}, errors.New("deferred metadata differs from original RocketMQ wire")
	}
	if err := Persist(ctx, tx, Source{Provider: message.Provider, Topic: message.Topic, Channel: message.Channel, TransportMessageID: message.TransportID, ApplicationID: message.ID, PhysicalTopic: message.PhysicalTopic, Wire: message.Wire, Metadata: encodedMetadata}); err != nil {
		return [32]byte{}, err
	}
	return Identity(message.Provider, message.Topic, message.Channel, message.TransportID), nil
}

// LoadRocketMQDelivery reads one exact immutable evidence identity in the
// caller's claim transaction. Attempts/timestamp belong to the deferred row.
func LoadRocketMQDelivery(ctx context.Context, tx *sql.Tx, identity [32]byte) (rmtransport.Received, error) {
	if tx == nil || identity == ([32]byte{}) {
		return rmtransport.Received{}, errors.New("original transaction and evidence identity required")
	}
	var message rmtransport.Received
	var metadata []byte
	err := tx.QueryRowContext(ctx, `SELECT provider,topic_name,channel_name,transport_message_id,application_id,physical_topic,CASE WHEN OCTET_LENGTH(original_wire) BETWEEN 1 AND 262144 THEN original_wire ELSE NULL END,CASE WHEN OCTET_LENGTH(metadata_bytes)<=65535 THEN metadata_bytes ELSE NULL END FROM event_delivery_transport_evidence WHERE identity_hash=? FOR SHARE`, identity[:]).Scan(&message.Provider, &message.Topic, &message.Channel, &message.TransportID, &message.ID, &message.PhysicalTopic, &message.Wire, &metadata)
	if err != nil {
		return rmtransport.Received{}, err
	}
	if message.Provider != "rocketmq" || message.ID == "" || message.PhysicalTopic == "" || len(message.Wire) == 0 || len(metadata) == 0 || identity != Identity(message.Provider, message.Topic, message.Channel, message.TransportID) {
		return rmtransport.Received{}, errors.New("deferred transport evidence binding conflict")
	}
	envelope, recognized, err := legacy.Decode(message.Wire)
	if err != nil || !recognized || envelope.UUID != message.ID {
		return rmtransport.Received{}, errors.New("deferred original wire identity conflict")
	}
	encoded, err := json.Marshal(envelope.Metadata)
	if err != nil || !bytes.Equal(encoded, metadata) {
		return rmtransport.Received{}, errors.New("deferred original metadata conflict")
	}
	message.Payload, message.Metadata = envelope.Payload, envelope.Metadata
	return message, nil
}

func Identity(provider, topic, channel, transportID string) [32]byte {
	return sha256.Sum256([]byte(provider + "\x00" + topic + "\x00" + channel + "\x00" + transportID))
}

func Persist(ctx context.Context, tx *sql.Tx, r Source) error {
	e := r
	if tx == nil || e.PhysicalTopic == "" || len(e.Wire) == 0 || !json.Valid(e.Metadata) {
		return errors.New("RocketMQ original transport evidence required before ACK")
	}
	id := Identity(r.Provider, r.Topic, r.Channel, r.TransportMessageID)
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
