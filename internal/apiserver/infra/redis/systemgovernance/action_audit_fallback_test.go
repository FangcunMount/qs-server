package systemgovernance

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	app "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	"github.com/alicebob/miniredis/v2"
	redis "github.com/redis/go-redis/v9"
)

func TestActionAuditFallbackPersistsOnlyTerminalReplayWithoutTTL(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	builder := keyspace.NewBuilderWithNamespace("ops:runtime")
	store := NewActionAuditFallbackStore(client, builder)
	finishedAt := time.Now().UTC().Truncate(time.Millisecond)
	record := app.ActionAuditRecord{
		OrgID: 9, RequestID: "request-1", ActionID: "resilience.release_lock", Status: "ok", FinishedAt: finishedAt,
		ActorUserID: 77, Input: map[string]interface{}{"token": "must-not-be-stored"},
		Result: &app.ActionRunResult{RequestID: "request-1", ActionID: "resilience.release_lock", Status: "ok"},
	}
	if err := store.Put(context.Background(), record); err != nil {
		t.Fatalf("Put() error=%v", err)
	}
	key := builder.BuildGovernanceAuditReplayKey("9", "request-1")
	raw, err := mr.Get(key)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(raw, "must-not-be-stored") || strings.Contains(raw, "actor_user_id") || mr.TTL(key) != 0 {
		t.Fatalf("fallback raw=%s ttl=%s", raw, mr.TTL(key))
	}
	indexKey := builder.BuildGovernanceAuditReplayIndexKey()
	if count, err := client.ZCard(context.Background(), indexKey).Result(); err != nil || count != 1 {
		t.Fatalf("fallback index count=%d err=%v", count, err)
	}
	if err := client.ZRem(context.Background(), indexKey, key).Err(); err != nil {
		t.Fatal(err)
	}
	if err := store.Put(context.Background(), record); err != nil {
		t.Fatalf("idempotent Put() error=%v", err)
	}
	if count, err := client.ZCard(context.Background(), indexKey).Result(); err != nil || count != 1 {
		t.Fatalf("idempotent Put() did not repair index: count=%d err=%v", count, err)
	}
	conflicting := record
	conflicting.Status = "failed"
	if err := store.Put(context.Background(), conflicting); err == nil || !strings.Contains(err.Error(), "conflicts") {
		t.Fatalf("conflicting Put() error=%v", err)
	}
	replayed, exists, err := store.Load(context.Background(), 9, "request-1")
	if err != nil || !exists || replayed.ActionID != record.ActionID || replayed.Result == nil || !replayed.FinishedAt.Equal(finishedAt) {
		t.Fatalf("Load() record=%+v exists=%v err=%v", replayed, exists, err)
	}
	records, err := store.List(context.Background(), 100)
	if err != nil || len(records) != 1 {
		t.Fatalf("List() records=%+v err=%v", records, err)
	}
	if err := store.Delete(context.Background(), 9, "request-1"); err != nil {
		t.Fatalf("Delete() error=%v", err)
	}
	if _, exists, err := store.Load(context.Background(), 9, "request-1"); err != nil || exists {
		t.Fatalf("Load() after delete exists=%v err=%v", exists, err)
	}
	if count, err := client.ZCard(context.Background(), indexKey).Result(); err != nil || count != 0 {
		t.Fatalf("fallback index after delete count=%d err=%v", count, err)
	}
}

func TestActionAuditFallbackListsOldestFirstWithinLimitAndPrunesMissingValues(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	builder := keyspace.NewBuilderWithNamespace("ops:runtime")
	store := NewActionAuditFallbackStore(client, builder)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)
	newer := app.ActionAuditRecord{OrgID: 9, RequestID: "newer", ActionID: "resilience.release_lock", Status: "ok", FinishedAt: now.Add(time.Minute)}
	older := app.ActionAuditRecord{OrgID: 9, RequestID: "older", ActionID: "resilience.release_lock", Status: "ok", FinishedAt: now}
	if err := store.Put(ctx, newer); err != nil {
		t.Fatal(err)
	}
	if err := store.Put(ctx, older); err != nil {
		t.Fatal(err)
	}
	indexKey := builder.BuildGovernanceAuditReplayIndexKey()
	missingKey := builder.BuildGovernanceAuditReplayKey("9", "missing")
	if err := client.ZAdd(ctx, indexKey, redis.Z{Score: float64(now.Add(-time.Minute).UnixMilli()), Member: missingKey}).Err(); err != nil {
		t.Fatal(err)
	}

	records, err := store.List(ctx, 1)
	if err != nil || len(records) != 1 || records[0].RequestID != older.RequestID {
		t.Fatalf("first List() records=%+v err=%v", records, err)
	}
	if score, err := client.ZScore(ctx, indexKey, missingKey).Result(); !errors.Is(err, redis.Nil) || score != 0 {
		t.Fatalf("missing index member score=%v err=%v", score, err)
	}
}
