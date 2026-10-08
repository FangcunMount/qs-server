package standardoutbox

import (
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/event"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"github.com/FangcunMount/reliable-messaging/message"
	"github.com/FangcunMount/reliable-messaging/wire/legacy"
)

func TestPreparedReferenceMatchesExactWireAndNanosecondEventTime(t *testing.T) {
	cfg, err := eventcatalog.Load("../../../../configs/events.yaml")
	if err != nil {
		t.Fatal(err)
	}
	evt := event.Event[map[string]any]{BaseEvent: event.BaseEvent{ID: "reference-1", EventTypeValue: eventcatalog.AnswerSheetSubmitted, AggregateTypeValue: "AnswerSheet", AggregateIDValue: "101", OccurredAtValue: time.Date(2026, 10, 8, 0, 0, 0, 123456789, time.UTC)}, Data: map[string]any{"org_id": 7, "request_id": "final-enriched"}}
	resolver := eventcatalog.NewCatalog(cfg)
	ref, err := PrepareReference(evt, resolver, "api-server")
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := PrepareIntents([]event.DomainEvent{evt}, resolver, "api-server")
	if err != nil {
		t.Fatal(err)
	}
	if ReferenceFromMessage(prepared[0].Message) != ref {
		t.Fatal("prepared reference differs from staged message")
	}
	if _, err := VerifyReference(prepared[0].Message.Input(), ref.Fingerprint, ref); err != nil {
		t.Fatal(err)
	}
	// A valid SDK fingerprint alone must not validate a different organization.
	in := prepared[0].Message.Input()
	in.Scope = "org:8"
	bad, err := message.New(in)
	if err != nil {
		t.Fatal(err)
	}
	altered := ReferenceFromMessage(bad)
	if _, err := VerifyReference(in, altered.Fingerprint, altered); err == nil {
		t.Fatal("cross-organization envelope accepted")
	}
	// The standard profile still requires the current Revision2 wire contract.
	in = prepared[0].Message.Input()
	outer, _, _ := legacy.Decode(in.Payload)
	in.Payload, err = legacy.Encode(outer, legacy.Revision1)
	if err != nil {
		t.Fatal(err)
	}
	bad, err = message.New(in)
	if err != nil {
		t.Fatal(err)
	}
	altered = ReferenceFromMessage(bad)
	if _, err := VerifyReference(in, altered.Fingerprint, altered); err == nil {
		t.Fatal("old transport encoding accepted as standard evidence")
	}
}
