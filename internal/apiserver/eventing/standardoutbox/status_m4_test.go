//go:build reliable_messaging_m4

package standardoutbox

import (
	"testing"
	"time"
)

func TestStandardStatusKeepsQuarantineAndRejectsUnknownState(t *testing.T) {
	now := time.Date(2026, 9, 23, 10, 0, 0, 0, time.FixedZone("UTC+8", 8*3600))
	oldest := now.Add(-time.Hour)
	snapshot, err := BuildStatusSnapshot("mongo-domain-events", now, []StatusCount{{State: "quarantined", Count: 2, OldestCreatedAt: &oldest}})
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Buckets) != 4 || snapshot.Buckets[3].Status != "quarantined" || snapshot.Buckets[3].Count != 2 || snapshot.Buckets[3].OldestAgeSeconds != 3600 {
		t.Fatalf("standard status lost quarantine or age: %+v", snapshot)
	}
	for _, invalid := range []StatusCount{{State: "mystery", Count: 1, OldestCreatedAt: &oldest}, {State: "pending", Count: 1}} {
		if _, err := BuildStatusSnapshot("mongo-domain-events", now, []StatusCount{invalid}); err == nil {
			t.Fatalf("invalid status %+v was accepted", invalid)
		}
	}
}
