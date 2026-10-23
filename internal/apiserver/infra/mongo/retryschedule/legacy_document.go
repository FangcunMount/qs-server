package retryschedule

import (
	model "github.com/FangcunMount/qs-server/internal/apiserver/eventing/retryschedule"
	"github.com/FangcunMount/reliable-messaging/message"
	"go.mongodb.org/mongo-driver/bson"
	"time"
)

type legacyHandoffDocument struct {
	ID            bson.Raw      `bson:"_id"`
	Producer      string        `bson:"producer"`
	Message       string        `bson:"message_id"`
	Destination   string        `bson:"destination"`
	Event         string        `bson:"event_type"`
	Schema        string        `bson:"schema_version"`
	Scope         string        `bson:"scope"`
	Content       string        `bson:"content_type"`
	Occurred      string        `bson:"occurred_at"`
	Payload       []byte        `bson:"payload"`
	Fingerprint   []byte        `bson:"fingerprint"`
	State         string        `bson:"state"`
	Code          string        `bson:"last_error_code"`
	Due           time.Time     `bson:"next_attempt_at"`
	Version       uint64        `bson:"version"`
	Attempts      uint64        `bson:"attempt_count"`
	Failures      uint64        `bson:"failure_count"`
	Claim         bson.RawValue `bson:"claim_token"`
	Lease         bson.RawValue `bson:"lease_until"`
	Confirmation  bson.RawValue `bson:"transport_confirmed_at"`
	Replay        bson.RawValue `bson:"manual_replay_request_id"`
	ReplayVersion bson.RawValue `bson:"manual_replay_version"`
}

func (r legacyHandoffDocument) legacySource() model.LegacySource {
	return model.LegacySource{Input: message.Input{Producer: r.Producer, ID: r.Message, Destination: r.Destination, EventType: r.Event, SchemaVersion: r.Schema, Scope: r.Scope, ContentType: r.Content, OccurredAt: r.Occurred, Payload: r.Payload}, Fingerprint: r.Fingerprint, State: r.State, Code: r.Code, NextAttemptAt: r.Due, Version: r.Version, Attempts: r.Attempts, Failures: r.Failures, HasClaim: fieldPresent(r.Claim), HasLease: fieldPresent(r.Lease), HasConfirmation: fieldPresent(r.Confirmation), HasManualReplay: fieldPresent(r.Replay) || fieldPresent(r.ReplayVersion)}
}
