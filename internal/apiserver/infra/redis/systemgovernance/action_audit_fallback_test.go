package systemgovernance

import (
	"context"
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
		OrgID: 9, RequestID: "request-1", ActionID: "resilience.resume_queue", Status: "ok", FinishedAt: finishedAt,
		ActorUserID: 77, Input: map[string]interface{}{"token": "must-not-be-stored"},
		Result: &app.ActionRunResult{RequestID: "request-1", ActionID: "resilience.resume_queue", Status: "ok"},
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
}
