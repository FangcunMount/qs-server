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
	return NewIndex(client), mr, client
}

func TestClaimDueIDsAtomicallyPopsDueMembers(t *testing.T) {
	index, mr, _ := newTestReadyIndex(t)
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	ctx := context.Background()
	key := "outbox:ready:" + outboxpriority.BucketP0

	if err := index.Enqueue(ctx, eventcatalog.AnswerSheetSubmitted, "evt-due-1", now.Add(-time.Minute)); err != nil {
		t.Fatalf("Enqueue due: %v", err)
	}
	if err := index.Enqueue(ctx, eventcatalog.AnswerSheetSubmitted, "evt-due-2", now); err != nil {
		t.Fatalf("Enqueue due 2: %v", err)
	}
	if err := index.Enqueue(ctx, eventcatalog.AnswerSheetSubmitted, "evt-future", now.Add(time.Hour)); err != nil {
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
		if err := index.Enqueue(ctx, eventcatalog.AnswerSheetSubmitted, eventID, now.Add(-time.Duration(i)*time.Second)); err != nil {
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
		if err := index.Enqueue(ctx, eventcatalog.AnswerSheetSubmitted, eventID, now); err != nil {
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
