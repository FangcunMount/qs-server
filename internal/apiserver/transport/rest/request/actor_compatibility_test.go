package request

import "testing"

func TestGetTesteeByProfileIDRequestCanonicalProfileID(t *testing.T) {
	req := &GetTesteeByProfileIDRequest{IAMChildID: "123"}
	if got := req.CanonicalProfileID(); got != "123" {
		t.Fatalf("expected legacy iam_child_id to be used, got %q", got)
	}

	req.ProfileID = "456"
	if got := req.CanonicalProfileID(); got != "456" {
		t.Fatalf("expected profile_id to take precedence, got %q", got)
	}
}
