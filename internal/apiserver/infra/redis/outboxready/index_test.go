package outboxready

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/outboxpriority"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newTestReadyIndex(t *testing.T) (*Index, *miniredis.Miniredis, *redis.Client) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return NewIndex(client, StoreMongoDomainEvents), mr, client
}

func TestClaimDueIDsAtomicallyPopsDueMembers(t *testing.T) {
	index, mr, _ := newTestReadyIndex(t)
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	ctx := context.Background()
	key := "outbox:ready:" + StoreMongoDomainEvents + ":" + outboxpriority.BucketP0

	if err := index.Enqueue(ctx, eventcatalog.AnswerSheetSubmitted, "evt-due-1", now.Add(-time.Minute), now.Add(-time.Minute)); err != nil {
		t.Fatalf("Enqueue due: %v", err)
	}
	if err := index.Enqueue(ctx, eventcatalog.AnswerSheetSubmitted, "evt-due-2", now, now); err != nil {
		t.Fatalf("Enqueue due 2: %v", err)
	}
	if err := index.Enqueue(ctx, eventcatalog.AnswerSheetSubmitted, "evt-future", now.Add(time.Hour), now.Add(time.Hour)); err != nil {
		t.Fatalf("Enqueue future: %v", err)
	}

	ids, err := index.ClaimDueIDs(ctx, outboxpriority.BucketP0, 10, now)
	if err != nil {
		t.Fatalf("ClaimDueIDs: %v", err)
	}
	if len(ids) != 2 || ids[0] != "evt-due-1" || ids[1] != "evt-due-2" {
		t.Fatalf("claimed ids = %#v, want [evt-due-1 evt-due-2]", ids)
	}
	remaining, err := mr.ZMembers(key)
	if err != nil {
		t.Fatalf("ZMembers: %v", err)
	}
	if len(remaining) != 1 || remaining[0] != "evt-future" {
		t.Fatalf("remaining zset = %#v, want only evt-future", remaining)
	}

	again, err := index.ClaimDueIDs(ctx, outboxpriority.BucketP0, 10, now)
	if err != nil {
		t.Fatalf("ClaimDueIDs again: %v", err)
	}
	if len(again) != 0 {
		t.Fatalf("second claim = %#v, want empty", again)
	}
}

func TestClaimDueIDsRespectsLimit(t *testing.T) {
	index, _, _ := newTestReadyIndex(t)
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		eventID := fmt.Sprintf("evt-%d", i)
		created := now.Add(-time.Duration(i) * time.Second)
		if err := index.Enqueue(ctx, eventcatalog.AnswerSheetSubmitted, eventID, now, created); err != nil {
			t.Fatalf("Enqueue %s: %v", eventID, err)
		}
	}

	ids, err := index.ClaimDueIDs(ctx, outboxpriority.BucketP0, 2, now)
	if err != nil {
		t.Fatalf("ClaimDueIDs: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("claimed len = %d, want 2", len(ids))
	}
}

func TestClaimDueIDsConcurrentClaimsDoNotOverlap(t *testing.T) {
	index, _, _ := newTestReadyIndex(t)
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	ctx := context.Background()

	for i := 0; i < 6; i++ {
		eventID := fmt.Sprintf("evt-%d", i)
		if err := index.Enqueue(ctx, eventcatalog.AnswerSheetSubmitted, eventID, now, now); err != nil {
			t.Fatalf("Enqueue %s: %v", eventID, err)
		}
	}

	first, err := index.ClaimDueIDs(ctx, outboxpriority.BucketP0, 3, now)
	if err != nil {
		t.Fatalf("first ClaimDueIDs: %v", err)
	}
	second, err := index.ClaimDueIDs(ctx, outboxpriority.BucketP0, 3, now)
	if err != nil {
		t.Fatalf("second ClaimDueIDs: %v", err)
	}
	if len(first)+len(second) != 6 {
		t.Fatalf("claimed total = %d, want 6", len(first)+len(second))
	}
	seen := make(map[string]struct{}, 6)
	for _, batch := range [][]string{first, second} {
		for _, eventID := range batch {
			if _, ok := seen[eventID]; ok {
				t.Fatalf("event %q claimed twice", eventID)
			}
			seen[eventID] = struct{}{}
		}
	}
}

func TestClaimDueIDsNilIndexIsNoop(t *testing.T) {
	ids, err := (*Index)(nil).ClaimDueIDs(context.Background(), outboxpriority.BucketP0, 10, time.Now())
	if err != nil {
		t.Fatalf("ClaimDueIDs: %v", err)
	}
	if len(ids) != 0 {
		t.Fatalf("ids = %#v, want nil", ids)
	}
}

func TestClaimDueIDsIsolatedByStoreNamespace(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	mongoIndex := NewIndex(client, StoreMongoDomainEvents)
	mysqlIndex := NewIndex(client, StoreAssessmentMySQLOutbox)
	now := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)
	ctx := context.Background()

	if err := mongoIndex.Enqueue(ctx, eventcatalog.AnswerSheetSubmitted, "mongo-evt", now, now); err != nil {
		t.Fatalf("mongo Enqueue: %v", err)
	}
	if err := mysqlIndex.Enqueue(ctx, eventcatalog.AssessmentSubmitted, "mysql-evt", now, now); err != nil {
		t.Fatalf("mysql Enqueue: %v", err)
	}

	mongoIDs, err := mongoIndex.ClaimDueIDs(ctx, outboxpriority.BucketP0, 10, now)
	if err != nil {
		t.Fatalf("mongo ClaimDueIDs: %v", err)
	}
	if len(mongoIDs) != 1 || mongoIDs[0] != "mongo-evt" {
		t.Fatalf("mongo claimed = %#v, want [mongo-evt]", mongoIDs)
	}

	mysqlIDs, err := mysqlIndex.ClaimDueIDs(ctx, outboxpriority.BucketP0, 10, now)
	if err != nil {
		t.Fatalf("mysql ClaimDueIDs: %v", err)
	}
	if len(mysqlIDs) != 1 || mysqlIDs[0] != "mysql-evt" {
		t.Fatalf("mysql claimed = %#v, want [mysql-evt]", mysqlIDs)
	}
}

func TestClaimDueIDsOrdersByCreatedAtWhenDueEqual(t *testing.T) {
	index, _, _ := newTestReadyIndex(t)
	ctx := context.Background()
	due := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)

	// Enqueue newer first to prove claim order follows createdAt, not insertion order.
	if err := index.Enqueue(ctx, eventcatalog.AnswerSheetSubmitted, "evt-new", due, due.Add(-time.Minute)); err != nil {
		t.Fatalf("enqueue new: %v", err)
	}
	if err := index.Enqueue(ctx, eventcatalog.AnswerSheetSubmitted, "evt-old", due, due.Add(-3*time.Minute)); err != nil {
		t.Fatalf("enqueue old: %v", err)
	}
	if err := index.Enqueue(ctx, eventcatalog.AnswerSheetSubmitted, "evt-mid", due, due.Add(-2*time.Minute)); err != nil {
		t.Fatalf("enqueue mid: %v", err)
	}

	ids, err := index.ClaimDueIDs(ctx, outboxpriority.BucketP0, 10, due)
	if err != nil {
		t.Fatalf("ClaimDueIDs: %v", err)
	}
	if len(ids) != 3 || ids[0] != "evt-old" || ids[1] != "evt-mid" || ids[2] != "evt-new" {
		t.Fatalf("claimed = %#v, want FIFO [evt-old evt-mid evt-new]", ids)
	}
}
