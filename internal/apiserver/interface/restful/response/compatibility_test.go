package response

import "testing"

func TestLegacyIAMChildIDAlias(t *testing.T) {
	profileID := "123"
	if got := LegacyIAMChildIDAlias(&profileID); got == nil || *got != "123" {
		t.Fatalf("expected legacy alias to mirror profile_id")
	}
	if got := LegacyIAMChildIDAlias(nil); got != nil {
		t.Fatalf("expected nil alias when profile_id is nil")
	}
}
