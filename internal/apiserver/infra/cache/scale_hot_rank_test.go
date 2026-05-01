package cache

import (
	"context"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane/keyspace"
	"github.com/alicebob/miniredis/v2"
	redis "github.com/redis/go-redis/v9"
)

func TestRedisScaleHotRankRecordsAndReadsWindow(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.Local)
	store := NewRedisScaleHotRank(client, keyspace.NewBuilderWithNamespace("test"))
	store.now = func() time.Time { return now }

	ctx := context.Background()
	mustRecordScaleHotRank(t, store, ctx, "Q-A", now)
	mustRecordScaleHotRank(t, store, ctx, "Q-A", now)
	mustRecordScaleHotRank(t, store, ctx, "Q-B", now.AddDate(0, 0, -1))
	mustRecordScaleHotRank(t, store, ctx, "Q-C", now.AddDate(0, 0, -40))

	items, err := store.TopSubmissions(ctx, 30, 3)
	if err != nil {
		t.Fatalf("TopSubmissions() error = %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("TopSubmissions() len = %d, want 2: %+v", len(items), items)
	}
	if items[0].QuestionnaireCode != "Q-A" || items[0].Score != 2 {
		t.Fatalf("first item = %+v, want Q-A score 2", items[0])
	}
	if items[1].QuestionnaireCode != "Q-B" || items[1].Score != 1 {
		t.Fatalf("second item = %+v, want Q-B score 1", items[1])
	}
}

func TestRedisScaleHotRankHonorsSingleDayWindow(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.Local)
	store := NewRedisScaleHotRank(client, keyspace.NewBuilderWithNamespace("test"))
	store.now = func() time.Time { return now }

	ctx := context.Background()
	mustRecordScaleHotRank(t, store, ctx, "Q-A", now)
	mustRecordScaleHotRank(t, store, ctx, "Q-B", now.AddDate(0, 0, -1))

	items, err := store.TopSubmissions(ctx, 1, 3)
	if err != nil {
		t.Fatalf("TopSubmissions() error = %v", err)
	}
	if len(items) != 1 || items[0].QuestionnaireCode != "Q-A" {
		t.Fatalf("TopSubmissions() = %+v, want only Q-A", items)
	}
}

func mustRecordScaleHotRank(t *testing.T, store *RedisScaleHotRank, ctx context.Context, questionnaireCode string, submittedAt time.Time) {
	t.Helper()
	if err := store.RecordSubmission(ctx, questionnaireCode, submittedAt); err != nil {
		t.Fatalf("RecordSubmission(%s) error = %v", questionnaireCode, err)
	}
}
