package eventconfig

import "testing"

func TestEventTypesMatchConfig(t *testing.T) {
	cfg, err := Load("../../../configs/events.yaml")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	expected := make(map[string]struct{}, len(EventTypes()))
	for _, eventType := range EventTypes() {
		expected[eventType] = struct{}{}
	}

	for eventType := range cfg.Events {
		if _, ok := expected[eventType]; !ok {
			t.Fatalf("config contains unexpected event type %q", eventType)
		}
		delete(expected, eventType)
	}

	if len(expected) != 0 {
		t.Fatalf("event types missing from config: %#v", expected)
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
