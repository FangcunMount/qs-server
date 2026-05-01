package statistics

import (
	"context"
	"strings"
	"testing"
	"time"

	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	"github.com/FangcunMount/qs-server/internal/pkg/locklease"
)

func TestNormalizeDailyWindowDefaultsToRepairWindow(t *testing.T) {
	service := &syncService{repairWindowDays: 7}
	now := time.Date(2026, 4, 17, 10, 0, 0, 0, time.Local)

	start, end, err := service.normalizeDailyWindow(now, SyncDailyOptions{})
	if err != nil {
		t.Fatalf("normalizeDailyWindow returned error: %v", err)
	}
	if want := time.Date(2026, 4, 10, 0, 0, 0, 0, time.Local); !start.Equal(want) {
		t.Fatalf("unexpected start: got %s want %s", start, want)
	}
	if want := time.Date(2026, 4, 17, 0, 0, 0, 0, time.Local); !end.Equal(want) {
		t.Fatalf("unexpected end: got %s want %s", end, want)
	}
}

func TestNormalizeDailyWindowRejectsPartialRange(t *testing.T) {
	service := &syncService{repairWindowDays: 7}
	start := time.Date(2026, 4, 10, 0, 0, 0, 0, time.Local)

	if _, _, err := service.normalizeDailyWindow(time.Now(), SyncDailyOptions{StartDate: &start}); err == nil {
		t.Fatalf("expected error for partial date range")
	}
}

type syncCtxMarker struct{}

type syncWriterStub struct {
	dailyCalled       bool
	orgSnapshotCalled bool
	planCalled        bool
	txCtxSeen         bool
}

func (s *syncWriterStub) RebuildDailyStatistics(ctx context.Context, _ int64, _, _ time.Time) error {
	s.dailyCalled = true
	s.txCtxSeen = ctx.Value(syncCtxMarker{}) == true
	return nil
}

func (s *syncWriterStub) RebuildOrgSnapshotStatistics(ctx context.Context, _ int64, _ time.Time) error {
	s.orgSnapshotCalled = true
	s.txCtxSeen = ctx.Value(syncCtxMarker{}) == true
	return nil
}

func (s *syncWriterStub) RebuildPlanStatistics(ctx context.Context, _ int64) error {
	s.planCalled = true
	s.txCtxSeen = ctx.Value(syncCtxMarker{}) == true
	return nil
}

type lockManagerStub struct {
	acquired bool
	key      string
}

func (s *lockManagerStub) AcquireSpec(_ context.Context, spec locklease.Spec, key string, _ ...time.Duration) (*locklease.Lease, bool, error) {
	s.acquired = true
	s.key = key
	return &locklease.Lease{Key: spec.Identity(key).Key, Token: "lease"}, true, nil
}

func (s *lockManagerStub) ReleaseSpec(context.Context, locklease.Spec, string, *locklease.Lease) error {
	return nil
}

func TestSyncDailyStatisticsUsesTransactionContextWriter(t *testing.T) {
	writer := &syncWriterStub{}
	locker := &lockManagerStub{}
	runner := apptransaction.RunnerFunc(func(ctx context.Context, fn func(context.Context) error) error {
		return fn(context.WithValue(ctx, syncCtxMarker{}, true))
	})
	service := NewSyncServiceWithTransactionRunner(runner, writer, 7, locker)

	err := service.SyncDailyStatistics(context.Background(), 9, SyncDailyOptions{
		StartDate: timePtrForTest(time.Date(2026, 4, 10, 0, 0, 0, 0, time.Local)),
		EndDate:   timePtrForTest(time.Date(2026, 4, 12, 0, 0, 0, 0, time.Local)),
	})
	if err != nil {
		t.Fatalf("SyncDailyStatistics returned error: %v", err)
	}
	if !locker.acquired {
		t.Fatalf("expected sync lock to be acquired")
	}
	if !writer.dailyCalled || !writer.txCtxSeen {
		t.Fatalf("writer dailyCalled=%v txCtxSeen=%v, want both true", writer.dailyCalled, writer.txCtxSeen)
	}
}

func TestSyncOrgSnapshotStatisticsUsesSnapshotLockAndWriter(t *testing.T) {
	writer := &syncWriterStub{}
	locker := &lockManagerStub{}
	runner := apptransaction.RunnerFunc(func(ctx context.Context, fn func(context.Context) error) error {
		return fn(context.WithValue(ctx, syncCtxMarker{}, true))
	})
	service := NewSyncServiceWithTransactionRunner(runner, writer, 7, locker)

	if err := service.SyncOrgSnapshotStatistics(context.Background(), 9); err != nil {
		t.Fatalf("SyncOrgSnapshotStatistics returned error: %v", err)
	}
	if !writer.orgSnapshotCalled || !writer.txCtxSeen {
		t.Fatalf("writer orgSnapshotCalled=%v txCtxSeen=%v, want both true", writer.orgSnapshotCalled, writer.txCtxSeen)
	}
	if !strings.HasPrefix(locker.key, "statistics:org_snapshot:9:") {
		t.Fatalf("lock key = %q, want statistics:org_snapshot prefix", locker.key)
	}
}

func timePtrForTest(value time.Time) *time.Time {
	return &value
}
