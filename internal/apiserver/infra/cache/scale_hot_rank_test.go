package cache

import (
	"context"
	"testing"
	"time"

	scale "github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane/keyspace"
	"github.com/alicebob/miniredis/v2"
	redis "github.com/redis/go-redis/v9"
)

func TestRedisScaleHotRankRecordsAndReadsWindow(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.Local)
	store := NewRedisScaleHotRankProjection(client, keyspace.NewBuilderWithNamespace("test"))
	store.now = func() time.Time { return now }

	ctx := context.Background()
	mustProjectScaleHotRank(t, store, ctx, "evt-1", "Q-A", now)
	mustProjectScaleHotRank(t, store, ctx, "evt-2", "Q-A", now)
	mustProjectScaleHotRank(t, store, ctx, "evt-3", "Q-B", now.AddDate(0, 0, -1))
	mustProjectScaleHotRank(t, store, ctx, "evt-4", "Q-C", now.AddDate(0, 0, -40))

	items, err := store.Top(ctx, scaleHotRankQuery(30, 3))
	if err != nil {
		t.Fatalf("Top() error = %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("Top() len = %d, want 2: %+v", len(items), items)
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
	store := NewRedisScaleHotRankProjection(client, keyspace.NewBuilderWithNamespace("test"))
	store.now = func() time.Time { return now }

	ctx := context.Background()
	mustProjectScaleHotRank(t, store, ctx, "evt-1", "Q-A", now)
	mustProjectScaleHotRank(t, store, ctx, "evt-2", "Q-B", now.AddDate(0, 0, -1))

	items, err := store.Top(ctx, scaleHotRankQuery(1, 3))
	if err != nil {
		t.Fatalf("Top() error = %v", err)
	}
	if len(items) != 1 || items[0].QuestionnaireCode != "Q-A" {
		t.Fatalf("Top() = %+v, want only Q-A", items)
	}
}

func TestRedisScaleHotRankProjectionIsIdempotentByEventID(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.Local)
	store := NewRedisScaleHotRankProjection(client, keyspace.NewBuilderWithNamespace("test"))
	store.now = func() time.Time { return now }

	ctx := context.Background()
	mustProjectScaleHotRank(t, store, ctx, "evt-1", "Q-A", now)
	mustProjectScaleHotRank(t, store, ctx, "evt-1", "Q-A", now)
	mustProjectScaleHotRank(t, store, ctx, "evt-2", "Q-A", now)

	items, err := store.Top(ctx, scaleHotRankQuery(30, 3))
	if err != nil {
		t.Fatalf("Top() error = %v", err)
	}
	if len(items) != 1 || items[0].QuestionnaireCode != "Q-A" || items[0].Score != 2 {
		t.Fatalf("Top() = %+v, want Q-A score 2", items)
	}
	if !mr.Exists("test:scale:hot:{rank}:projected:evt-1") {
		t.Fatal("processed idempotency key was not written")
	}
}

func mustProjectScaleHotRank(t *testing.T, store *RedisScaleHotRankProjection, ctx context.Context, eventID, questionnaireCode string, submittedAt time.Time) {
	t.Helper()
	if err := store.ProjectSubmission(ctx, domainScaleHotRankFact(eventID, questionnaireCode, submittedAt)); err != nil {
		t.Fatalf("ProjectSubmission(%s, %s) error = %v", eventID, questionnaireCode, err)
	}
}

func domainScaleHotRankFact(eventID, questionnaireCode string, submittedAt time.Time) scale.ScaleHotRankSubmissionFact {
	return scale.ScaleHotRankSubmissionFact{
		EventID:           eventID,
		QuestionnaireCode: questionnaireCode,
		SubmittedAt:       submittedAt,
	}
}

func scaleHotRankQuery(windowDays, limit int) scale.ScaleHotRankQuery {
	return scale.ScaleHotRankQuery{
		WindowDays: windowDays,
		Limit:      limit,
	}
}
