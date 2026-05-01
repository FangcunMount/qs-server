package scheduler

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"

	cachegov "github.com/FangcunMount/qs-server/internal/apiserver/application/cachegovernance"
	statisticsApp "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	apiserveroptions "github.com/FangcunMount/qs-server/internal/apiserver/options"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/locklease/redisadapter"
)

type fakeStatisticsSyncService struct {
	mu        sync.Mutex
	calls     []string
	errByCall map[string]error
	started   chan string
	block     <-chan struct{}
}

func (f *fakeStatisticsSyncService) SyncDailyStatistics(ctx context.Context, orgID int64, _ statisticsApp.SyncDailyOptions) error {
	return f.recordCall(ctx, "daily", orgID)
}

func (f *fakeStatisticsSyncService) SyncOrgSnapshotStatistics(ctx context.Context, orgID int64) error {
	return f.recordCall(ctx, "org_snapshot", orgID)
}

func (f *fakeStatisticsSyncService) SyncPlanStatistics(ctx context.Context, orgID int64) error {
	return f.recordCall(ctx, "plan", orgID)
}

func (f *fakeStatisticsSyncService) recordCall(ctx context.Context, stage string, orgID int64) error {
	call := statisticsSyncCall(stage, orgID)

	f.mu.Lock()
	f.calls = append(f.calls, call)
	err := f.errByCall[call]
	started := f.started
	block := f.block
	f.mu.Unlock()

	if started != nil {
		select {
		case started <- call:
		default:
		}
	}
	if block != nil && stage == "daily" {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-block:
		}
	}
	return err
}

func (f *fakeStatisticsSyncService) callOrder() []string {
	f.mu.Lock()
	defer f.mu.Unlock()

	calls := make([]string, len(f.calls))
	copy(calls, f.calls)
	return calls
}

type fakeStatisticsWarmupCoordinator struct {
	mu       sync.Mutex
	orgIDs   []int64
	errByOrg map[int64]error
}

func (f *fakeStatisticsWarmupCoordinator) WarmStartup(context.Context) error { return nil }

func (f *fakeStatisticsWarmupCoordinator) HandleScalePublished(context.Context, string) error {
	return nil
}

func (f *fakeStatisticsWarmupCoordinator) HandleQuestionnairePublished(context.Context, string, string) error {
	return nil
}

func (f *fakeStatisticsWarmupCoordinator) HandleStatisticsSync(_ context.Context, orgID int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.orgIDs = append(f.orgIDs, orgID)
	return f.errByOrg[orgID]
}

func (f *fakeStatisticsWarmupCoordinator) HandleRepairComplete(context.Context, cachegov.RepairCompleteRequest) error {
	return nil
}

func (f *fakeStatisticsWarmupCoordinator) HandleManualWarmup(context.Context, cachegov.ManualWarmupRequest) (*cachegov.ManualWarmupResult, error) {
	return nil, nil
}

func (f *fakeStatisticsWarmupCoordinator) Snapshot() cachegov.WarmupStatusSnapshot {
	return cachegov.WarmupStatusSnapshot{}
}

func (f *fakeStatisticsWarmupCoordinator) calls() []int64 {
	f.mu.Lock()
	defer f.mu.Unlock()

	orgIDs := make([]int64, len(f.orgIDs))
	copy(orgIDs, f.orgIDs)
	return orgIDs
}

func TestNewStatisticsSyncRunner(t *testing.T) {
	opts := newTestStatisticsSyncOptions()
	syncService := &fakeStatisticsSyncService{}

	if runner := newStatisticsSyncRunnerWithHooks(
		&apiserveroptions.StatisticsSyncOptions{Enable: false},
		syncService,
		nil,
		&redisadapter.Manager{},
		newTestStatisticsLockBuilder(),
		func(context.Context, redisadapter.Spec, string, time.Duration) (*redisadapter.Lease, bool, error) {
			return &redisadapter.Lease{Key: "k", Token: "t"}, true, nil
		},
		func(context.Context, redisadapter.Spec, string, *redisadapter.Lease) error { return nil },
	); runner != nil {
		t.Fatalf("expected disabled statistics sync to return nil")
	}

	if runner := newStatisticsSyncRunnerWithHooks(
		opts,
		nil,
		nil,
		&redisadapter.Manager{},
		newTestStatisticsLockBuilder(),
		func(context.Context, redisadapter.Spec, string, time.Duration) (*redisadapter.Lease, bool, error) {
			return &redisadapter.Lease{Key: "k", Token: "t"}, true, nil
		},
		func(context.Context, redisadapter.Spec, string, *redisadapter.Lease) error { return nil },
	); runner != nil {
		t.Fatalf("expected nil sync service to return nil")
	}

	if runner := newStatisticsSyncRunnerWithHooks(
		opts,
		syncService,
		nil,
		nil,
		newTestStatisticsLockBuilder(),
		func(context.Context, redisadapter.Spec, string, time.Duration) (*redisadapter.Lease, bool, error) {
			return &redisadapter.Lease{Key: "k", Token: "t"}, true, nil
		},
		func(context.Context, redisadapter.Spec, string, *redisadapter.Lease) error { return nil },
	); runner != nil {
		t.Fatalf("expected nil lock manager to return nil")
	}

	invalid := newTestStatisticsSyncOptions()
	invalid.RunAt = "bad"
	if runner := newStatisticsSyncRunnerWithHooks(
		invalid,
		syncService,
		nil,
		&redisadapter.Manager{},
		newTestStatisticsLockBuilder(),
		func(context.Context, redisadapter.Spec, string, time.Duration) (*redisadapter.Lease, bool, error) {
			return &redisadapter.Lease{Key: "k", Token: "t"}, true, nil
		},
		func(context.Context, redisadapter.Spec, string, *redisadapter.Lease) error { return nil },
	); runner != nil {
		t.Fatalf("expected invalid run_at to return nil")
	}
}

func TestStatisticsSyncRunnerLockKeyUsesLockNamespace(t *testing.T) {
	runner := newStatisticsSyncRunnerWithHooks(
		newTestStatisticsSyncOptions(),
		&fakeStatisticsSyncService{},
		nil,
		&redisadapter.Manager{},
		newTestStatisticsLockBuilder(),
		func(context.Context, redisadapter.Spec, string, time.Duration) (*redisadapter.Lease, bool, error) {
			return &redisadapter.Lease{Key: "k", Token: "t"}, true, nil
		},
		func(context.Context, redisadapter.Spec, string, *redisadapter.Lease) error { return nil },
	)
	if got := runner.lockKey(); got != "apiserver-test:cache:lock:qs:statistics-sync:test" {
		t.Fatalf("unexpected lock key: %s", got)
	}
}

func TestStatisticsSyncRunnerRunOnceUsesConfiguredLockOverride(t *testing.T) {
	syncService := &fakeStatisticsSyncService{}
	opts := newTestStatisticsSyncOptions()
	opts.LockKey = "qs:statistics-sync:custom"
	opts.LockTTL = 2 * time.Hour

	var gotSpec redisadapter.Spec
	var gotKey string
	var gotTTL time.Duration
	runner := newStatisticsSyncRunnerWithHooks(
		opts,
		syncService,
		nil,
		&redisadapter.Manager{},
		newTestStatisticsLockBuilder(),
		func(_ context.Context, spec redisadapter.Spec, key string, ttl time.Duration) (*redisadapter.Lease, bool, error) {
			gotSpec = spec
			gotKey = key
			gotTTL = ttl
			return &redisadapter.Lease{Key: "lock-key", Token: "token"}, true, nil
		},
		func(context.Context, redisadapter.Spec, string, *redisadapter.Lease) error { return nil },
	)

	if err := runner.runOnce(context.Background()); err != nil {
		t.Fatalf("runOnce returned error: %v", err)
	}
	if gotSpec.Name != redisadapter.Specs.StatisticsSyncLeader.Name {
		t.Fatalf("spec.name = %q, want %q", gotSpec.Name, redisadapter.Specs.StatisticsSyncLeader.Name)
	}
	if gotKey != opts.LockKey {
		t.Fatalf("key = %q, want %q", gotKey, opts.LockKey)
	}
	if gotTTL != opts.LockTTL {
		t.Fatalf("ttl = %s, want %s", gotTTL, opts.LockTTL)
	}
}

func TestStatisticsSyncRunnerRunOnceSchedulesEachOrgInOrder(t *testing.T) {
	lock := &fakeSchedulerLockManager{}
	syncService := &fakeStatisticsSyncService{}
	warmup := &fakeStatisticsWarmupCoordinator{}

	runner := newStatisticsSyncRunnerWithHooks(
		newTestStatisticsSyncOptions(11, 22),
		syncService,
		warmup,
		&redisadapter.Manager{},
		newTestStatisticsLockBuilder(),
		lock.acquire,
		lock.release,
	)

	if err := runner.runOnce(context.Background()); err != nil {
		t.Fatalf("runOnce returned error: %v", err)
	}

	wantCalls := []string{
		statisticsSyncCall("daily", 11),
		statisticsSyncCall("org_snapshot", 11),
		statisticsSyncCall("plan", 11),
		statisticsSyncCall("daily", 22),
		statisticsSyncCall("org_snapshot", 22),
		statisticsSyncCall("plan", 22),
	}
	if got := syncService.callOrder(); len(got) != len(wantCalls) {
		t.Fatalf("unexpected number of calls: got %v want %v", got, wantCalls)
	} else {
		for i := range wantCalls {
			if got[i] != wantCalls[i] {
				t.Fatalf("unexpected call order: got %v want %v", got, wantCalls)
			}
		}
	}
	if got := warmup.calls(); len(got) != 2 || got[0] != 11 || got[1] != 22 {
		t.Fatalf("unexpected warmup calls: %v", got)
	}
	if lock.releases() != 1 {
		t.Fatalf("expected lock release once, got %d", lock.releases())
	}
}

func TestStatisticsSyncRunnerRunOnceContinuesAfterOrgFailure(t *testing.T) {
	lock := &fakeSchedulerLockManager{}
	syncService := &fakeStatisticsSyncService{
		errByCall: map[string]error{
			statisticsSyncCall("daily", 2): errors.New("daily failed"),
		},
	}
	warmup := &fakeStatisticsWarmupCoordinator{}

	runner := newStatisticsSyncRunnerWithHooks(
		newTestStatisticsSyncOptions(1, 2, 3),
		syncService,
		warmup,
		&redisadapter.Manager{},
		newTestStatisticsLockBuilder(),
		lock.acquire,
		lock.release,
	)

	if err := runner.runOnce(context.Background()); err != nil {
		t.Fatalf("runOnce returned error: %v", err)
	}

	wantCalls := []string{
		statisticsSyncCall("daily", 1),
		statisticsSyncCall("org_snapshot", 1),
		statisticsSyncCall("plan", 1),
		statisticsSyncCall("daily", 2),
		statisticsSyncCall("daily", 3),
		statisticsSyncCall("org_snapshot", 3),
		statisticsSyncCall("plan", 3),
	}
	if got := syncService.callOrder(); len(got) != len(wantCalls) {
		t.Fatalf("unexpected number of calls: got %v want %v", got, wantCalls)
	} else {
		for i := range wantCalls {
			if got[i] != wantCalls[i] {
				t.Fatalf("unexpected call order: got %v want %v", got, wantCalls)
			}
		}
	}
	if got := warmup.calls(); len(got) != 2 || got[0] != 1 || got[1] != 3 {
		t.Fatalf("unexpected warmup calls: %v", got)
	}
}

func TestStatisticsSyncRunnerRunOnceSkipsWhenLockNotAcquired(t *testing.T) {
	syncService := &fakeStatisticsSyncService{}
	runner := newStatisticsSyncRunnerWithHooks(
		newTestStatisticsSyncOptions(),
		syncService,
		nil,
		&redisadapter.Manager{},
		newTestStatisticsLockBuilder(),
		func(context.Context, redisadapter.Spec, string, time.Duration) (*redisadapter.Lease, bool, error) {
			return nil, false, nil
		},
		func(context.Context, redisadapter.Spec, string, *redisadapter.Lease) error { return nil },
	)

	if err := runner.runOnce(context.Background()); err != nil {
		t.Fatalf("runOnce returned error: %v", err)
	}
	if got := syncService.callOrder(); len(got) != 0 {
		t.Fatalf("expected no scheduler calls when lock not acquired, got %v", got)
	}
}

func TestStatisticsSyncRunnerMultiInstanceOnlyOneExecutes(t *testing.T) {
	lock := &fakeSchedulerLockManager{}
	block := make(chan struct{})
	started := make(chan string, 1)

	syncService1 := &fakeStatisticsSyncService{started: started, block: block}
	syncService2 := &fakeStatisticsSyncService{}
	opts := newTestStatisticsSyncOptions()

	runner1 := newStatisticsSyncRunnerWithHooks(
		opts,
		syncService1,
		nil,
		&redisadapter.Manager{},
		newTestStatisticsLockBuilder(),
		lock.acquire,
		lock.release,
	)
	runner2 := newStatisticsSyncRunnerWithHooks(
		opts,
		syncService2,
		nil,
		&redisadapter.Manager{},
		newTestStatisticsLockBuilder(),
		lock.acquire,
		lock.release,
	)

	errCh := make(chan error, 1)
	go func() {
		errCh <- runner1.runOnce(context.Background())
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatalf("first runner did not start in time")
	}

	if err := runner2.runOnce(context.Background()); err != nil {
		t.Fatalf("second runner returned error: %v", err)
	}

	close(block)

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("first runner returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("first runner did not complete in time")
	}

	if calls := syncService1.callOrder(); len(calls) == 0 {
		t.Fatalf("expected first runner to execute")
	}
	if calls := syncService2.callOrder(); len(calls) != 0 {
		t.Fatalf("expected second runner to skip execution, got %v", calls)
	}
}

func newTestStatisticsSyncOptions(orgIDs ...int64) *apiserveroptions.StatisticsSyncOptions {
	if len(orgIDs) == 0 {
		orgIDs = []int64{1}
	}
	return &apiserveroptions.StatisticsSyncOptions{
		Enable:           true,
		OrgIDs:           orgIDs,
		RunAt:            "00:30",
		RepairWindowDays: 7,
		LockKey:          "qs:statistics-sync:test",
		LockTTL:          30 * time.Minute,
	}
}

func newTestStatisticsLockBuilder() *keyspace.Builder {
	return keyspace.NewBuilderWithNamespace(
		keyspace.ComposeNamespace("apiserver-test", "cache:lock"),
	)
}

func statisticsSyncCall(stage string, orgID int64) string {
	return stage + ":" + strconv.FormatInt(orgID, 10)
}
