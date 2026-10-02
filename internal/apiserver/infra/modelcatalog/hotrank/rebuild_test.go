package hotrank

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/hotrank"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	redis "github.com/redis/go-redis/v9"
)

func rebuildFixture(t *testing.T) (*RedisScaleHotRankProjection, *miniredis.Miniredis, RebuildSnapshot) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	r := NewRedisScaleHotRankProjection(client, keyspace.NewBuilderWithNamespace("rebuild-test"))
	now := time.Date(2026, 10, 2, 12, 0, 0, 0, hotRankTimezone)
	r.now = func() time.Time { return now }
	facts := []port.SubmissionFact{{EventID: uuid.NewString(), QuestionnaireCode: "Q-A", SubmittedAt: now}, {EventID: uuid.NewString(), QuestionnaireCode: "Q-A", SubmittedAt: now}, {EventID: uuid.NewString(), QuestionnaireCode: "Q-B", SubmittedAt: now}}
	hashes := map[string]string{}
	for _, f := range facts {
		hashes[f.EventID] = strings.Repeat("ab", 32)
	}
	return r, mr, RebuildSnapshot{Day: "20261002", CapturedAt: now, Facts: facts, OriginalFingerprints: hashes, Complete: true}
}
func acquireFixture(t *testing.T, r *RedisScaleHotRankProjection, day string) RebuildLease {
	t.Helper()
	lease, err := r.AcquireRebuild(context.Background(), day, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	return lease
}
func TestDayRebuildRestoresAbsoluteCountsAndLateOriginalEventDoesNotIncrement(t *testing.T) {
	r, _, source := rebuildFixture(t)
	ctx := context.Background()
	daily := r.keys.BuildScaleHotDailyKey(source.Day)
	if err := r.client.ZAdd(ctx, daily, redis.Z{Score: 99, Member: "Q-A"}, redis.Z{Score: 8, Member: "stale"}).Err(); err != nil {
		t.Fatal(err)
	}
	lease := acquireFixture(t, r, source.Day)
	receipt, err := r.ApplyDayRebuild(ctx, lease, source, "test-operator")
	if err != nil {
		t.Fatal(err)
	}
	if receipt.FactCount != 3 || receipt.Counts["Q-A"] != 2 || receipt.Counts["Q-B"] != 1 {
		t.Fatalf("source receipt=%+v", receipt)
	}
	for _, f := range source.Facts {
		if err := r.ProjectSubmission(ctx, f); err != nil {
			t.Fatal(err)
		}
	}
	items, err := r.Top(ctx, port.Query{WindowDays: 1, Limit: 5})
	if err != nil || len(items) != 2 || items[0].Score != 2 || items[1].Score != 1 {
		t.Fatalf("rank=%+v err=%v", items, err)
	}
	late := port.SubmissionFact{EventID: uuid.NewString(), QuestionnaireCode: "Q-A", SubmittedAt: r.now()}
	if err := r.ProjectSubmission(ctx, late); err != nil {
		t.Fatal(err)
	}
	repeated, err := r.ApplyDayRebuild(ctx, lease, source, "test-operator")
	if err != nil || repeated.Fingerprint != receipt.Fingerprint {
		t.Fatalf("receipt repeat=%+v err=%v", repeated, err)
	}
	if count := r.client.ZScore(ctx, daily, "Q-A").Val(); count != 3 {
		t.Fatalf("repeat overwrote later submission: %v", count)
	}
	if _, err := r.AcquireRebuild(ctx, source.Day, lease.RequestID); !errors.Is(err, ErrRebuildAlreadyApplied) {
		t.Fatalf("reacquire applied request=%v", err)
	}
}
func TestRebuildFencesRealtimeProjectionAndOtherOperators(t *testing.T) {
	r, _, source := rebuildFixture(t)
	ctx := context.Background()
	lease := acquireFixture(t, r, source.Day)
	if err := r.ProjectSubmission(ctx, source.Facts[0]); !errors.Is(err, ErrRebuildInProgress) {
		t.Fatalf("live projection during rebuild=%v", err)
	}
	if _, err := r.AcquireRebuild(ctx, source.Day, uuid.NewString()); !errors.Is(err, ErrRebuildInProgress) {
		t.Fatalf("parallel rebuild=%v", err)
	}
	if r.client.Exists(ctx, r.keys.BuildScaleHotProjectedKey(source.Facts[0].EventID)).Val() != 0 {
		t.Fatal("blocked live event was marked processed")
	}
	if err := r.ReleaseRebuild(ctx, lease); err != nil {
		t.Fatal(err)
	}
	if err := r.ProjectSubmission(ctx, source.Facts[0]); err != nil {
		t.Fatal(err)
	}
}
func TestExpiredRebuildCannotOverwriteNewLeaseOrReleaseIt(t *testing.T) {
	r, mr, source := rebuildFixture(t)
	ctx := context.Background()
	old := acquireFixture(t, r, source.Day)
	mr.FastForward(rebuildLeaseTTL + time.Second)
	current := acquireFixture(t, r, source.Day)
	if _, err := r.ApplyDayRebuild(ctx, old, source, "test"); !errors.Is(err, ErrRebuildLeaseLost) {
		t.Fatalf("stale write=%v", err)
	}
	if err := r.ReleaseRebuild(ctx, old); err != nil {
		t.Fatal(err)
	}
	if got := r.client.Get(ctx, r.keys.BuildScaleHotDailyKey(source.Day)+":rebuild-lock").Val(); got != current.Token {
		t.Fatal("stale owner released replacement lease")
	}
	if _, err := r.ApplyDayRebuild(ctx, current, source, "test"); err != nil {
		t.Fatal(err)
	}
}
func TestRebuildDetectsWrongTypeBeforeAnyDailyOrProcessedWrites(t *testing.T) {
	r, _, source := rebuildFixture(t)
	ctx := context.Background()
	daily := r.keys.BuildScaleHotDailyKey(source.Day)
	original := r.keys.BuildScaleHotProjectedKey(source.Facts[0].EventID)
	_ = r.client.ZAdd(ctx, daily, redis.Z{Score: 7, Member: "Q-A"}).Err()
	_ = r.client.LPush(ctx, original, "foreign").Err()
	lease := acquireFixture(t, r, source.Day)
	if _, err := r.ApplyDayRebuild(ctx, lease, source, "test"); !errors.Is(err, ErrRebuildConflict) {
		t.Fatalf("wrong type=%v", err)
	}
	if r.client.ZScore(ctx, daily, "Q-A").Val() != 7 || r.client.Exists(ctx, r.keys.BuildScaleHotProjectedKey(source.Facts[1].EventID)).Val() != 0 {
		t.Fatal("validation error changed existing state")
	}
}
func TestAppliedRequestCannotBeReusedWithChangedSource(t *testing.T) {
	r, _, source := rebuildFixture(t)
	ctx := context.Background()
	lease := acquireFixture(t, r, source.Day)
	if _, err := r.ApplyDayRebuild(ctx, lease, source, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.ApplyDayRebuild(ctx, lease, source, "different-operator"); !errors.Is(err, ErrRebuildConflict) {
		t.Fatalf("conflicting operator=%v", err)
	}
	source.Facts[0].QuestionnaireCode = "Q-conflict"
	if _, err := r.ApplyDayRebuild(ctx, lease, source, "test"); !errors.Is(err, ErrRebuildConflict) {
		t.Fatalf("conflicting request=%v", err)
	}
}
func TestRebuildRejectsIncompleteOversizedOrPreLeaseSource(t *testing.T) {
	for _, scenario := range []string{"incomplete", "oversized", "pre-lease", "missing-original"} {
		t.Run(scenario, func(t *testing.T) {
			r, _, source := rebuildFixture(t)
			ctx := context.Background()
			lease := acquireFixture(t, r, source.Day)
			switch scenario {
			case "incomplete":
				source.Complete = false
			case "oversized":
				source.Facts = make([]port.SubmissionFact, MaxRebuildFacts+1)
			case "pre-lease":
				source.CapturedAt = source.CapturedAt.Add(-time.Second)
			case "missing-original":
				delete(source.OriginalFingerprints, source.Facts[0].EventID)
			}
			if _, err := r.ApplyDayRebuild(ctx, lease, source, "test"); err == nil {
				t.Fatal("invalid source accepted")
			}
			if r.client.Exists(ctx, r.keys.BuildScaleHotDailyKey(source.Day)).Val() != 0 {
				t.Fatal("invalid source wrote rank")
			}
		})
	}
}
func TestHotRankBucketsUseUTCPlusEightRegardlessOfHostZone(t *testing.T) {
	r, _, _ := rebuildFixture(t)
	utc := time.Date(2026, 10, 1, 16, 5, 0, 0, time.UTC)
	r.now = func() time.Time { return utc }
	fact := port.SubmissionFact{EventID: uuid.NewString(), QuestionnaireCode: "Q", SubmittedAt: utc}
	if err := r.ProjectSubmission(context.Background(), fact); err != nil {
		t.Fatal(err)
	}
	if r.client.ZScore(context.Background(), r.keys.BuildScaleHotDailyKey("20261002"), "Q").Val() != 1 {
		t.Fatal("UTC host placed submission in previous business day")
	}
	items, err := r.Top(context.Background(), port.Query{WindowDays: 1, Limit: 1})
	if err != nil || len(items) != 1 {
		t.Fatalf("UTC+8 read window=%+v %v", items, err)
	}
}
