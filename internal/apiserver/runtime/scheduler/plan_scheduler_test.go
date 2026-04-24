package scheduler

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	planApp "github.com/FangcunMount/qs-server/internal/apiserver/application/plan"
	apiserveroptions "github.com/FangcunMount/qs-server/internal/apiserver/options"
	"github.com/FangcunMount/qs-server/internal/pkg/rediskey"
	"github.com/FangcunMount/qs-server/internal/pkg/redislock"
)

type fakePlanCommandService struct {
	mu         sync.Mutex
	calls      []int64
	statsByOrg map[int64]planApp.TaskScheduleStats
	errByOrg   map[int64]error
	started    chan int64
	block      <-chan struct{}
}

func (f *fakePlanCommandService) SchedulePendingTasks(ctx context.Context, orgID int64, _ string) (*planApp.TaskScheduleResult, error) {
	f.mu.Lock()
	f.calls = append(f.calls, orgID)
	stats := f.statsByOrg[orgID]
	err := f.errByOrg[orgID]
	started := f.started
	block := f.block
	f.mu.Unlock()

	if started != nil {
		select {
		case started <- orgID:
		default:
		}
	}
	if block != nil {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-block:
		}
	}
	if err != nil {
		return nil, err
	}
	return &planApp.TaskScheduleResult{Stats: stats}, nil
}

func (f *fakePlanCommandService) callOrder() []int64 {
	f.mu.Lock()
	defer f.mu.Unlock()

	calls := make([]int64, len(f.calls))
	copy(calls, f.calls)
	return calls
}

type fakeSchedulerLockManager struct {
	mu           sync.Mutex
	locked       bool
	releaseCount int
}

func (m *fakeSchedulerLockManager) acquire(_ context.Context, _ redislock.Spec, _ string, _ time.Duration) (*redislock.Lease, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.locked {
		return nil, false, nil
	}
	m.locked = true
	return &redislock.Lease{Key: "lock-key", Token: "token"}, true, nil
}

func (m *fakeSchedulerLockManager) release(_ context.Context, _ redislock.Spec, _ string, _ *redislock.Lease) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.locked {
		m.locked = false
		m.releaseCount++
	}
	return nil
}

func (m *fakeSchedulerLockManager) releases() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.releaseCount
}

func TestNewPlanRunner(t *testing.T) {
	opts := newTestPlanSchedulerOptions()
	command := &fakePlanCommandService{}

	if runner := newPlanRunnerWithHooks(
		&apiserveroptions.PlanSchedulerOptions{Enable: false},
		&redislock.Manager{},
		command,
		newTestPlanLockBuilder(),
		func(context.Context, redislock.Spec, string, time.Duration) (*redislock.Lease, bool, error) {
			return &redislock.Lease{Key: "k", Token: "t"}, true, nil
		},
		func(context.Context, redislock.Spec, string, *redislock.Lease) error { return nil },
	); runner != nil {
		t.Fatalf("expected disabled scheduler to return nil runner")
	}

	if runner := newPlanRunnerWithHooks(
		opts,
		nil,
		command,
		newTestPlanLockBuilder(),
		func(context.Context, redislock.Spec, string, time.Duration) (*redislock.Lease, bool, error) {
			return &redislock.Lease{Key: "k", Token: "t"}, true, nil
		},
		func(context.Context, redislock.Spec, string, *redislock.Lease) error { return nil },
	); runner != nil {
		t.Fatalf("expected nil lock manager to return nil runner")
	}

	if runner := newPlanRunnerWithHooks(
		opts,
		&redislock.Manager{},
		nil,
		newTestPlanLockBuilder(),
		func(context.Context, redislock.Spec, string, time.Duration) (*redislock.Lease, bool, error) {
			return &redislock.Lease{Key: "k", Token: "t"}, true, nil
		},
		func(context.Context, redislock.Spec, string, *redislock.Lease) error { return nil },
	); runner != nil {
		t.Fatalf("expected nil command service to return nil runner")
	}
}

func TestPlanRunnerRunOnceSchedulesEachOrgInOrder(t *testing.T) {
	lock := &fakeSchedulerLockManager{}
	command := &fakePlanCommandService{
		statsByOrg: map[int64]planApp.TaskScheduleStats{
			11: {OpenedCount: 2, ExpiredCount: 1},
			22: {OpenedCount: 1, ExpiredCount: 3},
			33: {OpenedCount: 0, ExpiredCount: 2},
		},
		errByOrg: map[int64]error{},
	}

	runner := newPlanRunnerWithHooks(
		newTestPlanSchedulerOptions(11, 22, 33),
		&redislock.Manager{},
		command,
		newTestPlanLockBuilder(),
		lock.acquire,
		lock.release,
	)

	if err := runner.runOnce(context.Background()); err != nil {
		t.Fatalf("runOnce returned error: %v", err)
	}

	gotCalls := command.callOrder()
	wantCalls := []int64{11, 22, 33}
	if len(gotCalls) != len(wantCalls) {
		t.Fatalf("unexpected number of calls: got %v want %v", gotCalls, wantCalls)
	}
	for i := range wantCalls {
		if gotCalls[i] != wantCalls[i] {
			t.Fatalf("unexpected call order: got %v want %v", gotCalls, wantCalls)
		}
	}
	if lock.releases() != 1 {
		t.Fatalf("expected lock release once, got %d", lock.releases())
	}
}

func TestPlanRunnerRunOnceContinuesAfterOrgFailure(t *testing.T) {
	lock := &fakeSchedulerLockManager{}
	command := &fakePlanCommandService{
		statsByOrg: map[int64]planApp.TaskScheduleStats{
			1: {OpenedCount: 1},
			3: {OpenedCount: 2, ExpiredCount: 1},
		},
		errByOrg: map[int64]error{
			2: errors.New("schedule failed"),
		},
	}

	runner := newPlanRunnerWithHooks(
		newTestPlanSchedulerOptions(1, 2, 3),
		&redislock.Manager{},
		command,
		newTestPlanLockBuilder(),
		lock.acquire,
		lock.release,
	)

	if err := runner.runOnce(context.Background()); err != nil {
		t.Fatalf("runOnce returned error: %v", err)
	}

	gotCalls := command.callOrder()
	wantCalls := []int64{1, 2, 3}
	if len(gotCalls) != len(wantCalls) {
		t.Fatalf("unexpected number of calls: got %v want %v", gotCalls, wantCalls)
	}
}

func TestPlanRunnerRunOnceSkipsWhenLockNotAcquired(t *testing.T) {
	command := &fakePlanCommandService{}
	runner := newPlanRunnerWithHooks(
		newTestPlanSchedulerOptions(1, 2),
		&redislock.Manager{},
		command,
		newTestPlanLockBuilder(),
		func(context.Context, redislock.Spec, string, time.Duration) (*redislock.Lease, bool, error) {
			return nil, false, nil
		},
		func(context.Context, redislock.Spec, string, *redislock.Lease) error { return nil },
	)

	if err := runner.runOnce(context.Background()); err != nil {
		t.Fatalf("runOnce returned error: %v", err)
	}
	if calls := command.callOrder(); len(calls) != 0 {
		t.Fatalf("expected no scheduler calls when lock not acquired, got %v", calls)
	}
}

func TestPlanRunnerLockKeyUsesLockNamespace(t *testing.T) {
	runner := newPlanRunnerWithHooks(
		newTestPlanSchedulerOptions(1),
		&redislock.Manager{},
		&fakePlanCommandService{},
		newTestPlanLockBuilder(),
		func(context.Context, redislock.Spec, string, time.Duration) (*redislock.Lease, bool, error) {
			return &redislock.Lease{Key: "k", Token: "t"}, true, nil
		},
		func(context.Context, redislock.Spec, string, *redislock.Lease) error { return nil },
	)
	if got := runner.lockKey(); got != "apiserver-test:cache:lock:qs:plan-scheduler:test" {
		t.Fatalf("unexpected lock key: %s", got)
	}
}

func TestPlanRunnerRunOnceUsesConfiguredLockOverride(t *testing.T) {
	command := &fakePlanCommandService{}
	opts := newTestPlanSchedulerOptions(1)
	opts.LockKey = "qs:plan-scheduler:custom"
	opts.LockTTL = 90 * time.Second

	var gotSpec redislock.Spec
	var gotKey string
	var gotTTL time.Duration
	runner := newPlanRunnerWithHooks(
		opts,
		&redislock.Manager{},
		command,
		newTestPlanLockBuilder(),
		func(_ context.Context, spec redislock.Spec, key string, ttl time.Duration) (*redislock.Lease, bool, error) {
			gotSpec = spec
			gotKey = key
			gotTTL = ttl
			return &redislock.Lease{Key: "lock-key", Token: "token"}, true, nil
		},
		func(context.Context, redislock.Spec, string, *redislock.Lease) error { return nil },
	)

	if err := runner.runOnce(context.Background()); err != nil {
		t.Fatalf("runOnce returned error: %v", err)
	}
	if gotSpec.Name != redislock.Specs.PlanSchedulerLeader.Name {
		t.Fatalf("spec.name = %q, want %q", gotSpec.Name, redislock.Specs.PlanSchedulerLeader.Name)
	}
	if gotKey != opts.LockKey {
		t.Fatalf("key = %q, want %q", gotKey, opts.LockKey)
	}
	if gotTTL != opts.LockTTL {
		t.Fatalf("ttl = %s, want %s", gotTTL, opts.LockTTL)
	}
}

func TestPlanRunnerStartStopsOnContextCancel(t *testing.T) {
	started := make(chan int64, 1)
	command := &fakePlanCommandService{
		statsByOrg: map[int64]planApp.TaskScheduleStats{
			1: {OpenedCount: 1},
		},
		errByOrg: map[int64]error{},
		started:  started,
	}
	lock := &fakeSchedulerLockManager{}
	opts := newTestPlanSchedulerOptions(1)
	opts.InitialDelay = 5 * time.Millisecond
	opts.Interval = time.Hour

	runner := newPlanRunnerWithHooks(
		opts,
		&redislock.Manager{},
		command,
		newTestPlanLockBuilder(),
		lock.acquire,
		lock.release,
	)

	ctx, cancel := context.WithCancel(context.Background())
	runner.Start(ctx)

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatalf("scheduler did not execute first tick in time")
	}

	cancel()
	time.Sleep(20 * time.Millisecond)
}

func TestPlanRunnerMultiInstanceOnlyOneExecutes(t *testing.T) {
	lock := &fakeSchedulerLockManager{}
	block := make(chan struct{})
	started := make(chan int64, 1)

	command1 := &fakePlanCommandService{
		statsByOrg: map[int64]planApp.TaskScheduleStats{
			1: {OpenedCount: 1},
		},
		errByOrg: map[int64]error{},
		started:  started,
		block:    block,
	}
	command2 := &fakePlanCommandService{
		statsByOrg: map[int64]planApp.TaskScheduleStats{
			1: {OpenedCount: 1},
		},
		errByOrg: map[int64]error{},
	}

	opts := newTestPlanSchedulerOptions(1)
	runner1 := newPlanRunnerWithHooks(
		opts,
		&redislock.Manager{},
		command1,
		newTestPlanLockBuilder(),
		lock.acquire,
		lock.release,
	)
	runner2 := newPlanRunnerWithHooks(
		opts,
		&redislock.Manager{},
		command2,
		newTestPlanLockBuilder(),
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

	if calls := command1.callOrder(); len(calls) != 1 || calls[0] != 1 {
		t.Fatalf("unexpected first runner calls: %v", calls)
	}
	if calls := command2.callOrder(); len(calls) != 0 {
		t.Fatalf("expected second runner to skip execution, got calls %v", calls)
	}
}

func newTestPlanSchedulerOptions(orgIDs ...int64) *apiserveroptions.PlanSchedulerOptions {
	if len(orgIDs) == 0 {
		orgIDs = []int64{1}
	}
	return &apiserveroptions.PlanSchedulerOptions{
		Enable:       true,
		OrgIDs:       orgIDs,
		InitialDelay: 0,
		Interval:     time.Minute,
		LockKey:      "qs:plan-scheduler:test",
		LockTTL:      30 * time.Second,
	}
}

func newTestPlanLockBuilder() *rediskey.Builder {
	return rediskey.NewBuilderWithNamespace(
		rediskey.ComposeNamespace("apiserver-test", "cache:lock"),
	)
}
