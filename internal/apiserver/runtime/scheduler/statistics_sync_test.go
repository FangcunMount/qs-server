package scheduler

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	statisticsApp "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	statisticsDomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	apiserveroptions "github.com/FangcunMount/qs-server/internal/apiserver/options"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/locklease"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/locklease/redisadapter"
)

type fakeStatisticsCoordinator struct {
	mu       sync.Mutex
	requests []statisticsApp.RunRequest
	errByOrg map[int64]error
}

func (f *fakeStatisticsCoordinator) Run(_ context.Context, request statisticsApp.RunRequest) (*statisticsApp.Run, error) {
	f.mu.Lock()
	f.requests = append(f.requests, request)
	err := f.errByOrg[request.OrgID]
	f.mu.Unlock()
	status := statisticsDomain.RunStatusSucceeded
	if err != nil {
		status = statisticsDomain.RunStatusFailed
	}
	return &statisticsApp.Run{OrgID: request.OrgID, Mode: request.Mode, Status: status}, err
}

func (f *fakeStatisticsCoordinator) calls() []statisticsApp.RunRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]statisticsApp.RunRequest(nil), f.requests...)
}

func TestNewStatisticsSyncRunnerRequiresCanonicalDependencies(t *testing.T) {
	opts := newTestStatisticsSyncOptions()
	coordinator := &fakeStatisticsCoordinator{}
	manager := &redisadapter.Manager{}

	if got := newStatisticsSyncRunnerWithHooks(&apiserveroptions.StatisticsSyncOptions{Enable: false}, coordinator, manager, newTestStatisticsLockBuilder(), acquireStatisticsTestLock, releaseStatisticsTestLock); got != nil {
		t.Fatal("disabled scheduler must not start")
	}
	if got := newStatisticsSyncRunnerWithHooks(opts, nil, manager, newTestStatisticsLockBuilder(), acquireStatisticsTestLock, releaseStatisticsTestLock); got != nil {
		t.Fatal("scheduler without coordinator must not start")
	}
	if got := newStatisticsSyncRunnerWithHooks(opts, coordinator, nil, newTestStatisticsLockBuilder(), acquireStatisticsTestLock, releaseStatisticsTestLock); got != nil {
		t.Fatal("scheduler without lock manager must not start")
	}
	invalid := newTestStatisticsSyncOptions()
	invalid.RunAt = "invalid"
	if got := newStatisticsSyncRunnerWithHooks(invalid, coordinator, manager, newTestStatisticsLockBuilder(), acquireStatisticsTestLock, releaseStatisticsTestLock); got != nil {
		t.Fatal("scheduler with invalid run_at must not start")
	}
}

func TestStatisticsSyncRunnerPublishesOneRunPerOrganization(t *testing.T) {
	coordinator := &fakeStatisticsCoordinator{}
	runner := newStatisticsSyncRunnerWithHooks(
		newTestStatisticsSyncOptions(), coordinator, &redisadapter.Manager{}, newTestStatisticsLockBuilder(),
		acquireStatisticsTestLock, releaseStatisticsTestLock,
	)
	runner.now = func() time.Time {
		return time.Date(2026, 7, 22, 1, 0, 0, 0, statisticsDomain.Shanghai)
	}

	if err := runner.runOnce(context.Background()); err != nil {
		t.Fatalf("runOnce() error = %v", err)
	}
	requests := coordinator.calls()
	if len(requests) != 2 {
		t.Fatalf("Run calls = %d, want 2", len(requests))
	}
	for index, request := range requests {
		if request.OrgID != int64(index+1) {
			t.Fatalf("request[%d].OrgID = %d", index, request.OrgID)
		}
		if request.Mode != statisticsDomain.RunModePublish || request.TriggerType != "scheduled" {
			t.Fatalf("request[%d] is not a scheduled publish: %+v", index, request)
		}
		if got := request.FromDate.Format("2006-01-02"); got != "2026-07-15" {
			t.Fatalf("request[%d].FromDate = %s", index, got)
		}
		if got := request.ToDate.Format("2006-01-02"); got != "2026-07-21" {
			t.Fatalf("request[%d].ToDate = %s", index, got)
		}
	}
}

func TestStatisticsSyncRunnerReportsPartialFailure(t *testing.T) {
	coordinator := &fakeStatisticsCoordinator{errByOrg: map[int64]error{2: errors.New("projection failed")}}
	runner := newStatisticsSyncRunnerWithHooks(
		newTestStatisticsSyncOptions(), coordinator, &redisadapter.Manager{}, newTestStatisticsLockBuilder(),
		acquireStatisticsTestLock, releaseStatisticsTestLock,
	)
	runner.now = func() time.Time {
		return time.Date(2026, 7, 22, 1, 0, 0, 0, statisticsDomain.Shanghai)
	}

	var partial *StatisticsSyncPartialError
	if err := runner.runOnce(context.Background()); !errors.As(err, &partial) {
		t.Fatalf("runOnce() error = %v, want partial error", err)
	}
	if partial.Summary.Succeeded != 1 || partial.Summary.Failed != 1 {
		t.Fatalf("summary = %+v", partial.Summary)
	}
	if got := len(coordinator.calls()); got != 2 {
		t.Fatalf("Run calls = %d, want scheduler to continue after one org fails", got)
	}
}

func TestStatisticsSyncRunnerUsesCanonicalLockNamespace(t *testing.T) {
	runner := newStatisticsSyncRunnerWithHooks(
		newTestStatisticsSyncOptions(), &fakeStatisticsCoordinator{}, &redisadapter.Manager{}, newTestStatisticsLockBuilder(),
		acquireStatisticsTestLock, releaseStatisticsTestLock,
	)
	if got := runner.lockKey(); got != "apiserver-test:cache:lock:qs:statistics-sync:test" {
		t.Fatalf("lock key = %q", got)
	}
}

func newTestStatisticsSyncOptions() *apiserveroptions.StatisticsSyncOptions {
	return &apiserveroptions.StatisticsSyncOptions{
		Enable:           true,
		OrgIDs:           []int64{1, 2},
		RunAt:            "00:30",
		RepairWindowDays: 7,
		LockKey:          "qs:statistics-sync:test",
		LockTTL:          30 * time.Second,
	}
}

func newTestStatisticsLockBuilder() *keyspace.Builder {
	return keyspace.NewBuilderWithNamespace(keyspace.ComposeNamespace("apiserver-test", "cache:lock"))
}

func acquireStatisticsTestLock(_ context.Context, _ locklease.Spec, key string, _ time.Duration) (*locklease.Lease, bool, error) {
	return &locklease.Lease{Key: key, Token: "test-token"}, true, nil
}

func releaseStatisticsTestLock(context.Context, locklease.Spec, string, *locklease.Lease) error {
	return nil
}
