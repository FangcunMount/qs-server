//go:build integration

package systemgovernance

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	app "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	redis "github.com/redis/go-redis/v9"
)

func TestActionAuditFallbackIndexAgainstRedis7(t *testing.T) {
	redisURL := os.Getenv("QS_SERVER_TEST_REDIS_URL")
	if redisURL == "" {
		t.Skip("QS_SERVER_TEST_REDIS_URL is required")
	}
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		t.Fatal(err)
	}
	client := redis.NewClient(opts)
	t.Cleanup(func() { _ = client.Close() })
	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Fatal(err)
	}

	namespace := fmt.Sprintf("integration:governance:%d", time.Now().UnixNano())
	builder := keyspace.NewBuilderWithNamespace(namespace)
	store := NewActionAuditFallbackStore(client, builder)
	record := app.ActionAuditRecord{
		OrgID: 9, RequestID: "request-1", ActionID: "resilience.release_lock",
		Status: "ok", FinishedAt: time.Now().UTC().Truncate(time.Millisecond),
	}
	valueKey := builder.BuildGovernanceAuditReplayKey("9", record.RequestID)
	indexKey := builder.BuildGovernanceAuditReplayIndexKey()
	t.Cleanup(func() { _ = client.Del(context.Background(), valueKey, indexKey).Err() })

	if err := store.Put(ctx, record); err != nil {
		t.Fatal(err)
	}
	records, err := store.List(ctx, 10)
	if err != nil || len(records) != 1 || records[0].RequestID != record.RequestID {
		t.Fatalf("List() records=%+v err=%v", records, err)
	}
	if err := store.Delete(ctx, record.OrgID, record.RequestID); err != nil {
		t.Fatal(err)
	}
	if count, err := client.ZCard(ctx, indexKey).Result(); err != nil || count != 0 {
		t.Fatalf("fallback index after delete count=%d err=%v", count, err)
	}
}
