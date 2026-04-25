package eventcatalog

import (
	"slices"
	"testing"
)

func TestLoadEventsYAMLAndCodeConstantsStayInSync(t *testing.T) {
	cfg, err := Load("../../../configs/events.yaml")
	if err != nil {
		t.Fatalf("Load events.yaml: %v", err)
	}

	if missing := ValidateEventTypes(cfg); len(missing) > 0 {
		t.Fatalf("code event constants missing from yaml: %v", missing)
	}
	for eventType := range cfg.Events {
		if !slices.Contains(EventTypes(), eventType) {
			t.Fatalf("yaml event %q missing from code constants", eventType)
		}
	}
}

func TestRemovedEventTypesAbsent(t *testing.T) {
	cfg, err := Load("../../../configs/events.yaml")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	removed := []string{
		"questionnaire.published",
		"questionnaire.unpublished",
		"questionnaire.archived",
		"scale.published",
		"scale.unpublished",
		"scale.updated",
		"scale.archived",
		"plan.created",
		"plan.testee_enrolled",
		"plan.testee_terminated",
		"plan.paused",
		"plan.resumed",
		"plan.canceled",
		"plan.finished",
		"report.exported",
	}

	for _, eventType := range removed {
		if _, ok := cfg.Events[eventType]; ok {
			t.Fatalf("removed event type %q still exists in config", eventType)
		}
	}
}

func TestCatalogQueriesTopicHandlerAndSubscription(t *testing.T) {
	cfg, err := Load("../../../configs/events.yaml")
	if err != nil {
		t.Fatalf("Load events.yaml: %v", err)
	}

	catalog := NewCatalog(cfg)
	topic, ok := catalog.GetTopicForEvent(AnswerSheetSubmitted)
	if !ok {
		t.Fatalf("GetTopicForEvent(%q) not found", AnswerSheetSubmitted)
	}
	if topic == "" {
		t.Fatalf("GetTopicForEvent(%q) returned empty topic", AnswerSheetSubmitted)
	}

	eventCfg, ok := catalog.GetEventConfig(AnswerSheetSubmitted)
	if !ok {
		t.Fatalf("GetEventConfig(%q) not found", AnswerSheetSubmitted)
	}
	if eventCfg.Handler == "" {
		t.Fatalf("GetEventConfig(%q) returned empty handler", AnswerSheetSubmitted)
	}
	if eventCfg.Delivery != DeliveryClassDurableOutbox {
		t.Fatalf("delivery = %q, want %q", eventCfg.Delivery, DeliveryClassDurableOutbox)
	}

	subscriptions := catalog.TopicSubscriptions()
	if len(subscriptions) == 0 {
		t.Fatalf("TopicSubscriptions returned none")
	}
	found := false
	for _, sub := range subscriptions {
		if sub.TopicName == topic && slices.Contains(sub.EventTypes, AnswerSheetSubmitted) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("TopicSubscriptions did not include %q on %q", AnswerSheetSubmitted, topic)
	}
}

func TestCatalogDeliveryClass(t *testing.T) {
	cfg, err := Load("../../../configs/events.yaml")
	if err != nil {
		t.Fatalf("Load events.yaml: %v", err)
	}

	catalog := NewCatalog(cfg)
	tests := []struct {
		eventType string
		delivery  DeliveryClass
		durable   bool
	}{
		{QuestionnaireChanged, DeliveryClassBestEffort, false},
		{ScaleChanged, DeliveryClassBestEffort, false},
		{TaskOpened, DeliveryClassBestEffort, false},
		{TaskCompleted, DeliveryClassBestEffort, false},
		{TaskExpired, DeliveryClassBestEffort, false},
		{TaskCanceled, DeliveryClassBestEffort, false},
		{AnswerSheetSubmitted, DeliveryClassDurableOutbox, true},
		{AssessmentSubmitted, DeliveryClassDurableOutbox, true},
		{AssessmentInterpreted, DeliveryClassDurableOutbox, true},
		{AssessmentFailed, DeliveryClassDurableOutbox, true},
		{ReportGenerated, DeliveryClassDurableOutbox, true},
		{FootprintEntryOpened, DeliveryClassDurableOutbox, true},
		{FootprintIntakeConfirmed, DeliveryClassDurableOutbox, true},
		{FootprintTesteeProfileCreated, DeliveryClassDurableOutbox, true},
		{FootprintCareRelationshipEstablished, DeliveryClassDurableOutbox, true},
		{FootprintCareRelationshipTransferred, DeliveryClassDurableOutbox, true},
		{FootprintAnswerSheetSubmitted, DeliveryClassDurableOutbox, true},
		{FootprintAssessmentCreated, DeliveryClassDurableOutbox, true},
		{FootprintReportGenerated, DeliveryClassDurableOutbox, true},
	}

	if len(tests) != len(EventTypes()) {
		t.Fatalf("delivery test cases = %d, event types = %d", len(tests), len(EventTypes()))
	}

	for _, tt := range tests {
		delivery, ok := catalog.GetDeliveryClass(tt.eventType)
		if !ok {
			t.Fatalf("GetDeliveryClass(%q) not found", tt.eventType)
		}
		if delivery != tt.delivery {
			t.Fatalf("GetDeliveryClass(%q) = %q, want %q", tt.eventType, delivery, tt.delivery)
		}
		if durable := catalog.IsDurableOutbox(tt.eventType); durable != tt.durable {
			t.Fatalf("IsDurableOutbox(%q) = %v, want %v", tt.eventType, durable, tt.durable)
		}
	}
}

func TestEventsYAMLDeclaresDeliveryClassForEveryEvent(t *testing.T) {
	cfg, err := Load("../../../configs/events.yaml")
	if err != nil {
		t.Fatalf("Load events.yaml: %v", err)
	}
	for eventType, eventCfg := range cfg.Events {
		if !eventCfg.Delivery.Valid() {
			t.Fatalf("event %q delivery = %q, want valid delivery class", eventType, eventCfg.Delivery)
		}
	}
}

func TestParseRejectsDanglingTopicEmptyHandlerAndInvalidDelivery(t *testing.T) {
	t.Run("dangling topic", func(t *testing.T) {
		_, err := Parse([]byte(`
version: "1"
topics:
  known:
    name: known.topic
events:
  sample.created:
    topic: missing
    delivery: best_effort
    handler: sample_handler
`))
		if err == nil {
			t.Fatalf("Parse should reject event referencing unknown topic")
		}
	})

	t.Run("empty handler", func(t *testing.T) {
		_, err := Parse([]byte(`
version: "1"
topics:
  known:
    name: known.topic
events:
  sample.created:
    topic: known
    delivery: best_effort
`))
		if err == nil {
			t.Fatalf("Parse should reject empty handler")
		}
	})

	t.Run("empty delivery", func(t *testing.T) {
		_, err := Parse([]byte(`
version: "1"
topics:
  known:
    name: known.topic
events:
  sample.created:
    topic: known
    handler: sample_handler
`))
		if err == nil {
			t.Fatalf("Parse should reject empty delivery")
		}
	})

	t.Run("invalid delivery", func(t *testing.T) {
		_, err := Parse([]byte(`
version: "1"
topics:
  known:
    name: known.topic
events:
  sample.created:
    topic: known
    delivery: exactly_once
    handler: sample_handler
`))
		if err == nil {
			t.Fatalf("Parse should reject invalid delivery")
		}
	})
}
