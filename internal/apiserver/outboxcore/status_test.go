package outboxcore

import (
	"testing"
	"time"

	outboxport "github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
)

func TestBuildStatusSnapshotReturnsCanonicalUnfinishedBuckets(t *testing.T) {
	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	oldestPending := now.Add(-2 * time.Minute)
	oldestPublishing := now.Add(30 * time.Second)

	snapshot := BuildStatusSnapshot("mysql", now, []StatusObservation{
		{Status: StatusPending, Count: 3, OldestCreatedAt: &oldestPending},
		{Status: StatusPublished, Count: 10, OldestCreatedAt: &oldestPending},
		{Status: StatusPublishing, Count: 1, OldestCreatedAt: &oldestPublishing},
	})

	if snapshot.Store != "mysql" {
		t.Fatalf("store = %q, want mysql", snapshot.Store)
	}
	if !snapshot.GeneratedAt.Equal(now) {
		t.Fatalf("generated_at = %v, want %v", snapshot.GeneratedAt, now)
	}
	if len(snapshot.Buckets) != 3 {
		t.Fatalf("buckets = %#v, want pending/failed/publishing", snapshot.Buckets)
	}

	assertBucket(t, snapshot.Buckets[0], StatusPending, 3, 120)
	assertBucket(t, snapshot.Buckets[1], StatusFailed, 0, 0)
	assertBucket(t, snapshot.Buckets[2], StatusPublishing, 1, 0)
}

func TestBuildStatusSnapshotEmptyIncludesZeroBuckets(t *testing.T) {
	snapshot := BuildStatusSnapshot("mongo", time.Time{}, nil)

	if snapshot.GeneratedAt.IsZero() {
		t.Fatalf("generated_at should default to now")
	}
	if len(snapshot.Buckets) != 3 {
		t.Fatalf("buckets = %#v, want three zero buckets", snapshot.Buckets)
	}
	for _, bucket := range snapshot.Buckets {
		if bucket.Count != 0 || bucket.OldestAgeSeconds != 0 || bucket.OldestCreatedAt != nil {
			t.Fatalf("bucket = %#v, want zero bucket", bucket)
		}
	}
}

func assertBucket(t *testing.T, bucket outboxport.StatusBucket, status string, count int64, ageSeconds float64) {
	t.Helper()
	if bucket.Status != status {
		t.Fatalf("status = %q, want %q", bucket.Status, status)
	}
	if bucket.Count != count {
		t.Fatalf("count = %d, want %d", bucket.Count, count)
	}
	if bucket.OldestAgeSeconds != ageSeconds {
		t.Fatalf("age = %v, want %v", bucket.OldestAgeSeconds, ageSeconds)
	}
}
