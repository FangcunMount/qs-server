package cache

import "testing"

func TestAddNamespace(t *testing.T) {
	ApplyNamespace("dev")
	defer ApplyNamespace("")

	key := addNamespace("scale:ABC")
	if key != "dev:scale:ABC" {
		t.Fatalf("expected namespaced key dev:scale:ABC, got %s", key)
	}

	ApplyNamespace("")
	key = addNamespace("scale:ABC")
	if key != "scale:ABC" {
		t.Fatalf("expected key without namespace, got %s", key)
	}
}
