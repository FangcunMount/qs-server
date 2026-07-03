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
	if replay.Enabled || !replay.Planned {
		t.Fatalf("events.replay_pending = %#v, want disabled planned action", replay)
	}
	if manual.InputSchema == nil {
		t.Fatal("cache.manual_warmup input_schema is nil")
	}
}
