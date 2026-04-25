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

func TestParseRejectsDanglingTopicAndEmptyHandler(t *testing.T) {
	t.Run("dangling topic", func(t *testing.T) {
		_, err := Parse([]byte(`
version: "1"
topics:
  known:
    name: known.topic
events:
  sample.created:
    topic: missing
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
`))
		if err == nil {
			t.Fatalf("Parse should reject empty handler")
		}
	})
}
