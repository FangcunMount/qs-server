package systemgovernance

import "testing"

func TestActionRegistryIncludesEnabledAndPlannedActions(t *testing.T) {
	actions := NewActionRegistry().List()
	if len(actions) < 6 {
		t.Fatalf("actions len = %d, want at least 6", len(actions))
	}
	var manual, replay ActionDescriptor
	for _, item := range actions {
		switch item.ID {
		case "cache.manual_warmup":
			manual = item
		case "events.replay_pending":
			replay = item
		}
	}
	if !manual.Enabled || manual.Planned {
		t.Fatalf("cache.manual_warmup = %#v, want enabled and not planned", manual)
	}
	if !replay.Enabled || replay.Planned || !replay.RequiresConfirmation || replay.InputSchema == nil {
		t.Fatalf("events.replay_pending = %#v, want enabled governed action", replay)
	}
	if manual.InputSchema == nil {
		t.Fatal("cache.manual_warmup input_schema is nil")
	}
}

func TestRetryManualActionsEmergencySwitch(t *testing.T) {
	registry := NewActionRegistry(map[string]bool{"retry.manual_actions": false})
	for _, id := range []string{"evaluation.retry", "evaluation.force_retry", "interpretation.retry", "interpretation.force_retry", "events.replay_pending", "events.replay_delivery"} {
		action, ok := registry.Get(id)
		if !ok || action.Enabled {
			t.Fatalf("action %s = %#v, want disabled", id, action)
		}
	}
	cacheAction, _ := registry.Get("cache.manual_warmup")
	if !cacheAction.Enabled {
		t.Fatal("retry switch must not disable unrelated cache actions")
	}
}
