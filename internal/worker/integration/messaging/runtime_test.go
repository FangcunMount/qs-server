package messaging

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	basemessaging "github.com/FangcunMount/component-base/pkg/messaging"
)

type fakeDispatcher struct {
	eventType string
	payload   []byte
	err       error
	calls     int
}

func (d *fakeDispatcher) DispatchEvent(_ context.Context, eventType string, payload []byte) error {
	d.calls++
	d.eventType = eventType
	d.payload = payload
	return d.err
}

func TestDispatchHandlerUsesMetadataEventTypeFirst(t *testing.T) {
	dispatcher := &fakeDispatcher{}
	msg := basemessaging.NewMessage("msg-1", []byte("not-json"))
	msg.Metadata["event_type"] = "metadata.event"
	ackCount := 0
	msg.SetAckFunc(func() error {
		ackCount++
		return nil
	})

	handler := createDispatchHandler(testLogger(), dispatcher, "topic")
	if err := handler(context.Background(), msg); err != nil {
		t.Fatalf("handler: %v", err)
	}

	if dispatcher.eventType != "metadata.event" {
		t.Fatalf("eventType = %q, want metadata.event", dispatcher.eventType)
	}
	if ackCount != 1 {
		t.Fatalf("ackCount = %d, want 1", ackCount)
	}
}

func TestDispatchHandlerFallsBackToPayloadEnvelope(t *testing.T) {
	dispatcher := &fakeDispatcher{}
	payload := []byte(`{"id":"evt-1","eventType":"payload.event","occurredAt":"2026-04-25T00:00:00Z","aggregateType":"Sample","aggregateID":"sample-1","data":{"ok":true}}`)
	msg := basemessaging.NewMessage("msg-1", payload)

	handler := createDispatchHandler(testLogger(), dispatcher, "topic")
	if err := handler(context.Background(), msg); err != nil {
		t.Fatalf("handler: %v", err)
	}

	if dispatcher.eventType != "payload.event" {
		t.Fatalf("eventType = %q, want payload.event", dispatcher.eventType)
	}
	if msg.Metadata["event_type"] != "payload.event" {
		t.Fatalf("metadata event_type = %q, want payload.event", msg.Metadata["event_type"])
	}
}

func TestMessageEventExtractorUsesMetadataBeforePayload(t *testing.T) {
	msg := basemessaging.NewMessage("msg-1", []byte("not-json"))
	msg.Metadata["event_type"] = "metadata.event"

	eventType, err := (MessageEventExtractor{}).Extract(msg)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if eventType != "metadata.event" {
		t.Fatalf("eventType = %q, want metadata.event", eventType)
	}
}

func TestMessageSettlementPolicyAckSuccess(t *testing.T) {
	msg := basemessaging.NewMessage("msg-1", []byte("{}"))
	ackCount := 0
	msg.SetAckFunc(func() error {
		ackCount++
		return nil
	})

	policy := MessageSettlementPolicy{logger: testLogger(), topic: "topic"}
	if err := policy.AckSuccess(msg); err != nil {
		t.Fatalf("AckSuccess: %v", err)
	}
	if ackCount != 1 {
		t.Fatalf("ackCount = %d, want 1", ackCount)
	}
}

func TestDispatchHandlerAcksInvalidPayloadWithoutDispatch(t *testing.T) {
	dispatcher := &fakeDispatcher{}
	msg := basemessaging.NewMessage("msg-1", []byte("not-json"))
	ackCount := 0
	msg.SetAckFunc(func() error {
		ackCount++
		return nil
	})

	handler := createDispatchHandler(testLogger(), dispatcher, "topic")
	if err := handler(context.Background(), msg); err != nil {
		t.Fatalf("handler: %v", err)
	}

	if dispatcher.calls != 0 {
		t.Fatalf("dispatch calls = %d, want 0", dispatcher.calls)
	}
	if ackCount != 1 {
		t.Fatalf("ackCount = %d, want 1", ackCount)
	}
}

func TestDispatchHandlerNacksOnDispatchError(t *testing.T) {
	wantErr := errors.New("dispatch failed")
	dispatcher := &fakeDispatcher{err: wantErr}
	msg := basemessaging.NewMessage("msg-1", []byte(`{}`))
	msg.Metadata["event_type"] = "metadata.event"
	nackCount := 0
	msg.SetNackFunc(func() error {
		nackCount++
		return nil
	})

	handler := createDispatchHandler(testLogger(), dispatcher, "topic")
	if err := handler(context.Background(), msg); !errors.Is(err, wantErr) {
		t.Fatalf("handler error = %v, want %v", err, wantErr)
	}

	if nackCount != 1 {
		t.Fatalf("nackCount = %d, want 1", nackCount)
	}
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
