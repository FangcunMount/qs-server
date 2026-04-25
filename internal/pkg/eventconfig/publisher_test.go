package eventconfig

import (
	"context"
	"testing"

	"github.com/FangcunMount/component-base/pkg/messaging"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/pkg/event"
)

type capturedPublisher struct {
	topic string
	msg   *messaging.Message
}

func (p *capturedPublisher) Publish(_ context.Context, _ string, _ []byte) error {
	return nil
}

func (p *capturedPublisher) PublishMessage(_ context.Context, topic string, msg *messaging.Message) error {
	p.topic = topic
	p.msg = msg
	return nil
}

func (p *capturedPublisher) Close() error {
	return nil
}

func TestRoutingPublisherUsesExplicitCatalogAndMetadata(t *testing.T) {
	cfg, err := Load("../../../configs/events.yaml")
	if err != nil {
		t.Fatalf("Load events.yaml: %v", err)
	}
	mq := &capturedPublisher{}
	publisher := NewRoutingPublisher(RoutingPublisherOptions{
		Catalog:     eventcatalog.NewCatalog(cfg),
		MQPublisher: mq,
		Source:      "unit-test",
		Mode:        PublishModeMQ,
	})
	evt := event.New(AnswerSheetSubmitted, "AnswerSheet", "sheet-1", map[string]string{"id": "sheet-1"})

	if err := publisher.Publish(context.Background(), evt); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	if mq.topic == "" {
		t.Fatalf("topic was not captured")
	}
	if mq.msg == nil {
		t.Fatalf("message was not captured")
	}
	if mq.msg.Metadata["event_type"] != AnswerSheetSubmitted {
		t.Fatalf("event_type metadata = %q", mq.msg.Metadata["event_type"])
	}
	if mq.msg.Metadata["source"] != "unit-test" {
		t.Fatalf("source metadata = %q", mq.msg.Metadata["source"])
	}
	if len(mq.msg.Payload) == 0 {
		t.Fatalf("payload is empty")
	}
}

func TestRoutingPublisherRejectsUnknownEvent(t *testing.T) {
	publisher := NewRoutingPublisher(RoutingPublisherOptions{
		Catalog: eventcatalog.NewCatalog(&Config{
			Topics: map[string]TopicConfig{},
			Events: map[string]EventConfig{},
		}),
		Mode: PublishModeNop,
	})
	evt := event.New("unknown.event", "Unknown", "1", map[string]string{})

	if err := publisher.Publish(context.Background(), evt); err == nil {
		t.Fatalf("Publish should reject unknown event")
	}
}
