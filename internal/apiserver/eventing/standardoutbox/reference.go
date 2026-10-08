package standardoutbox

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/event"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	eventevidence "github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	"github.com/FangcunMount/reliable-messaging/message"
	domainwire "github.com/FangcunMount/reliable-messaging/wire/domain"
	"github.com/FangcunMount/reliable-messaging/wire/legacy"
)

func ReferenceFromMessage(msg message.Message) eventevidence.StandardReference {
	in := msg.Input()
	fingerprint := msg.Fingerprint()
	return eventevidence.StandardReference{EventID: in.ID, Producer: in.Producer, Destination: in.Destination, EventType: in.EventType,
		SchemaVersion: in.SchemaVersion, Scope: in.Scope, ContentType: in.ContentType, OccurredAt: in.OccurredAt, Fingerprint: hex.EncodeToString(fingerprint[:])}
}

func PrepareReference(evt event.DomainEvent, resolver eventcatalog.TopicResolver, source string) (eventevidence.StandardReference, error) {
	prepared, err := PrepareIntents([]event.DomainEvent{evt}, resolver, source)
	if err != nil {
		return eventevidence.StandardReference{}, err
	}
	ref := ReferenceFromMessage(prepared[0].Message)
	return ref, ref.Validate()
}

// VerifyReference validates both the immutable SDK identity and the exact
// Revision2 transport/domain envelopes. The caller additionally verifies its
// typed payload against frozen business facts.
func VerifyReference(in message.Input, rowFingerprint string, expected eventevidence.StandardReference) (*domainwire.Envelope, error) {
	if err := expected.Validate(); err != nil {
		return nil, err
	}
	msg, err := message.New(in)
	if err != nil {
		return nil, err
	}
	actual := ReferenceFromMessage(msg)
	if actual != expected || rowFingerprint != actual.Fingerprint {
		return nil, fmt.Errorf("standard event identity or fingerprint conflict")
	}
	outer, recognized, err := legacy.Decode(in.Payload)
	if err != nil || !recognized {
		return nil, fmt.Errorf("invalid standard transport envelope: recognized=%t error=%v", recognized, err)
	}
	encoded, err := legacy.Encode(outer, legacy.Revision2)
	if err != nil || !bytes.Equal(encoded, in.Payload) {
		return nil, fmt.Errorf("standard transport envelope is not the original Revision2 encoding")
	}
	inner, err := domainwire.DecodeEnvelope(outer.Payload)
	if err != nil {
		return nil, err
	}
	occurredAt, err := time.Parse(time.RFC3339Nano, in.OccurredAt)
	if err != nil || outer.UUID != in.ID || inner.ID != in.ID || inner.EventType != in.EventType || inner.AggregateType == "" || inner.AggregateID == "" ||
		!occurredAt.Equal(inner.OccurredAt) || outer.Metadata["event_type"] != inner.EventType || outer.Metadata["aggregate_type"] != inner.AggregateType ||
		outer.Metadata["aggregate_id"] != inner.AggregateID || outer.Metadata["source"] == "" || outer.Metadata["occurred_at"] != inner.OccurredAt.Format(domainwire.OccurredAtLayout) {
		return nil, fmt.Errorf("standard domain envelope identity conflict")
	}
	var payload struct {
		OrgID int64 `json:"org_id"`
	}
	if err := json.Unmarshal(inner.Data, &payload); err != nil || payload.OrgID <= 0 || in.Scope != fmt.Sprintf("org:%d", payload.OrgID) {
		return nil, fmt.Errorf("standard domain organization scope conflict")
	}
	return inner, nil
}
