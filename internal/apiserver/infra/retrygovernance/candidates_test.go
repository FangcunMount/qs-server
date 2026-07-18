package retrygovernance

import "testing"

func TestCandidateCursorRoundTripAndBounds(t *testing.T) {
	cursor := encodeCandidateCursor(125)
	offset, err := decodeCandidateCursor(cursor)
	if err != nil || offset != 125 {
		t.Fatalf("decode cursor = %d, %v; want 125, nil", offset, err)
	}
	for _, invalid := range []string{"%%%", encodeCandidateCursor(maxCandidateOffset + 1)} {
		if _, err := decodeCandidateCursor(invalid); err == nil {
			t.Fatalf("decodeCandidateCursor(%q) unexpectedly succeeded", invalid)
		}
	}
}
