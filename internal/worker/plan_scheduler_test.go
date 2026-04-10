package worker

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	planApp "github.com/FangcunMount/qs-server/internal/apiserver/application/plan"
	pb "github.com/FangcunMount/qs-server/internal/apiserver/interface/grpc/proto/internalapi"
	workerconfig "github.com/FangcunMount/qs-server/internal/worker/config"
	redis "github.com/redis/go-redis/v9"
)

type fakePlanSchedulerClient struct {
	mu         sync.Mutex
	calls      []int64
	statsByOrg map[int64]*pb.TaskScheduleStatsMessage
	errByOrg   map[int64]error
	started    chan int64
	block      <-chan struct{}
}

func (f *fakePlanSchedulerClient) SchedulePendingTasks(ctx context.Context, req *pb.SchedulePendingTasksRequest) (*pb.SchedulePendingTasksResponse, error) {
	f.mu.Lock()
	f.calls = append(f.calls, req.GetOrgId())
	stats := f.statsByOrg[req.GetOrgId()]
	err := f.errByOrg[req.GetOrgId()]
	started := f.started
	block := f.block
	f.mu.Unlock()

	if req.GetSource() != planApp.TaskSchedulerSourceBuiltin {
		return nil, errors.New("unexpected schedule source")
	}

	if started != nil {
		select {
		case started <- req.GetOrgId():
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
	if stats == nil {
		stats = &pb.TaskScheduleStatsMessage{}
	}
	return &pb.SchedulePendingTasksResponse{
		Stats: stats,
		Tasks: make([]*pb.TaskResultMessage, stats.GetOpenedCount()),
	}, nil
}

func (f *fakePlanSchedulerClient) callOrder() []int64 {
	f.mu.Lock()
	defer f.mu.Unlock()

	calls := make([]int64, len(f.calls))
	copy(calls, f.calls)
	return calls
}

type fakeWorkerLockManager struct {
	mu           sync.Mutex
	locked       bool
	releaseCount int
}

func (m *fakeWorkerLockManager) acquire(context.Context, string, time.Duration) (string, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.locked {
		return "", false, nil
	}
	m.locked = true
	return "token", true, nil
}

func (m *fakeWorkerLockManager) release(context.Context, string, string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.locked {
		m.locked = false
		m.releaseCount++
	}
	return nil
}

func (m *fakeWorkerLockManager) releases() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.releaseCount
}

func TestNewWorkerPlanSchedulerRunner(t *testing.T) {
	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:6379"})
	defer client.Close()

	opts := newTestWorkerPlanSchedulerConfig()
	planClient := &fakePlanSchedulerClient{}

	if runner := newWorkerPlanSchedulerRunnerWithHooks(
		&workerconfig.PlanSchedulerConfig{Enable: false},
		client,
		planClient,
		func(context.Context) error { return nil },
		func(context.Context, string, time.Duration) (string, bool, error) { return "token", true, nil },
		func(context.Context, string, string) error { return nil },
	); runner != nil {
		t.Fatalf("expected disabled scheduler to return nil runner")
	}

	if runner := newWorkerPlanSchedulerRunnerWithHooks(
		opts,
		nil,
		planClient,
		func(context.Context) error { return nil },
		func(context.Context, string, time.Duration) (string, bool, error) { return "token", true, nil },
		func(context.Context, string, string) error { return nil },
	); runner != nil {
		t.Fatalf("expected nil redis client to return nil runner")
	}

	if runner := newWorkerPlanSchedulerRunnerWithHooks(
		opts,
		client,
		nil,
		func(context.Context) error { return nil },
		func(context.Context, string, time.Duration) (string, bool, error) { return "token", true, nil },
		func(context.Context, string, string) error { return nil },
	); runner != nil {
		t.Fatalf("expected nil plan client to return nil runner")
	}

	if runner := newWorkerPlanSchedulerRunnerWithHooks(
		opts,
		client,
		planClient,
		func(context.Context) error { return errors.New("ping failed") },
		func(context.Context, string, time.Duration) (string, bool, error) { return "token", true, nil },
		func(context.Context, string, string) error { return nil },
	); runner != nil {
		t.Fatalf("expected ping failure to return nil runner")
	}
}

func TestWorkerPlanSchedulerRunOnceSchedulesEachOrgInOrder(t *testing.T) {
	lock := &fakeWorkerLockManager{}
	client := &fakePlanSchedulerClient{
		statsByOrg: map[int64]*pb.TaskScheduleStatsMessage{
			11: {OpenedCount: 2, ExpiredCount: 1},
			22: {OpenedCount: 1, ExpiredCount: 3},
			33: {OpenedCount: 0, ExpiredCount: 2},
		},
		errByOrg: map[int64]error{},
	}

	runner := newWorkerPlanSchedulerRunnerWithHooks(
		newTestWorkerPlanSchedulerConfig(11, 22, 33),
		redis.NewClient(&redis.Options{Addr: "127.0.0.1:6379"}),
		client,
		func(context.Context) error { return nil },
		lock.acquire,
		lock.release,
	)

	if err := runner.runOnce(context.Background()); err != nil {
		t.Fatalf("runOnce returned error: %v", err)
	}

	gotCalls := client.callOrder()
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

func TestWorkerPlanSchedulerRunOnceContinuesAfterOrgFailure(t *testing.T) {
	lock := &fakeWorkerLockManager{}
	client := &fakePlanSchedulerClient{
		statsByOrg: map[int64]*pb.TaskScheduleStatsMessage{
			1: {OpenedCount: 1},
			3: {OpenedCount: 2, ExpiredCount: 1},
		},
		errByOrg: map[int64]error{
			2: errors.New("schedule failed"),
		},
	}

	runner := newWorkerPlanSchedulerRunnerWithHooks(
		newTestWorkerPlanSchedulerConfig(1, 2, 3),
		redis.NewClient(&redis.Options{Addr: "127.0.0.1:6379"}),
		client,
		func(context.Context) error { return nil },
		lock.acquire,
		lock.release,
	)

	if err := runner.runOnce(context.Background()); err != nil {
		t.Fatalf("runOnce returned error: %v", err)
	}

	gotCalls := client.callOrder()
	wantCalls := []int64{1, 2, 3}
	if len(gotCalls) != len(wantCalls) {
		t.Fatalf("unexpected number of calls: got %v want %v", gotCalls, wantCalls)
	}
}

func TestWorkerPlanSchedulerRunOnceSkipsWhenLockNotAcquired(t *testing.T) {
	client := &fakePlanSchedulerClient{}
	runner := newWorkerPlanSchedulerRunnerWithHooks(
		newTestWorkerPlanSchedulerConfig(1, 2),
		redis.NewClient(&redis.Options{Addr: "127.0.0.1:6379"}),
		client,
		func(context.Context) error { return nil },
		func(context.Context, string, time.Duration) (string, bool, error) { return "", false, nil },
		func(context.Context, string, string) error { return nil },
	)

	if err := runner.runOnce(context.Background()); err != nil {
		t.Fatalf("runOnce returned error: %v", err)
	}
	if calls := client.callOrder(); len(calls) != 0 {
		t.Fatalf("expected no scheduler calls when lock not acquired, got %v", calls)
	}
}

func TestWorkerPlanSchedulerStartStopsOnContextCancel(t *testing.T) {
	started := make(chan int64, 1)
	client := &fakePlanSchedulerClient{
		statsByOrg: map[int64]*pb.TaskScheduleStatsMessage{
			1: {OpenedCount: 1},
		},
		errByOrg: map[int64]error{},
		started:  started,
	}
	lock := &fakeWorkerLockManager{}
	opts := newTestWorkerPlanSchedulerConfig(1)
	opts.InitialDelay = 5 * time.Millisecond
	opts.Interval = time.Hour

	runner := newWorkerPlanSchedulerRunnerWithHooks(
		opts,
		redis.NewClient(&redis.Options{Addr: "127.0.0.1:6379"}),
		client,
		func(context.Context) error { return nil },
		lock.acquire,
		lock.release,
	)

	ctx, cancel := context.WithCancel(context.Background())
	done := runner.start(ctx)

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatalf("scheduler did not execute first tick in time")
	}

	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("scheduler did not stop after context cancellation")
	}
}

func TestNextWorkerPlanSchedulerTickTimeAlignsWholeMinuteIntervals(t *testing.T) {
	loc := time.FixedZone("UTC+8", 8*60*60)
	tests := []struct {
		name     string
		now      time.Time
		interval time.Duration
		want     time.Time
	}{
		{
			name:     "one minute aligns to next minute boundary",
			now:      time.Date(2026, 4, 10, 18, 59, 50, 123000000, loc),
			interval: time.Minute,
			want:     time.Date(2026, 4, 10, 19, 0, 0, 0, loc),
		},
		{
			name:     "exact boundary advances by one interval",
			now:      time.Date(2026, 4, 10, 19, 0, 0, 0, loc),
			interval: time.Minute,
			want:     time.Date(2026, 4, 10, 19, 1, 0, 0, loc),
		},
		{
			name:     "five minutes aligns to local five minute boundary",
			now:      time.Date(2026, 4, 10, 18, 57, 12, 0, loc),
			interval: 5 * time.Minute,
			want:     time.Date(2026, 4, 10, 19, 0, 0, 0, loc),
		},
		{
			name:     "two hours rolls to next day boundary when needed",
			now:      time.Date(2026, 4, 10, 23, 59, 59, 0, loc),
			interval: 2 * time.Hour,
			want:     time.Date(2026, 4, 11, 0, 0, 0, 0, loc),
		},
		{
			name:     "non whole minute interval keeps relative cadence",
			now:      time.Date(2026, 4, 10, 18, 59, 50, 0, loc),
			interval: 90 * time.Second,
			want:     time.Date(2026, 4, 10, 19, 1, 20, 0, loc),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := nextWorkerPlanSchedulerTickTime(tt.now, tt.interval)
			if !got.Equal(tt.want) {
				t.Fatalf("unexpected next tick time: got %s want %s", got.Format(time.RFC3339), tt.want.Format(time.RFC3339))
			}
		})
	}
}

func TestWorkerPlanSchedulerMultiInstanceOnlyOneExecutes(t *testing.T) {
	lock := &fakeWorkerLockManager{}
	block := make(chan struct{})
	started := make(chan int64, 1)

	client1 := &fakePlanSchedulerClient{
		statsByOrg: map[int64]*pb.TaskScheduleStatsMessage{
			1: {OpenedCount: 1},
		},
		errByOrg: map[int64]error{},
		started:  started,
		block:    block,
	}
	client2 := &fakePlanSchedulerClient{
		statsByOrg: map[int64]*pb.TaskScheduleStatsMessage{
			1: {OpenedCount: 1},
		},
		errByOrg: map[int64]error{},
	}

	opts := newTestWorkerPlanSchedulerConfig(1)
	runner1 := newWorkerPlanSchedulerRunnerWithHooks(
		opts,
		redis.NewClient(&redis.Options{Addr: "127.0.0.1:6379"}),
		client1,
		func(context.Context) error { return nil },
		lock.acquire,
		lock.release,
	)
	runner2 := newWorkerPlanSchedulerRunnerWithHooks(
		opts,
		redis.NewClient(&redis.Options{Addr: "127.0.0.1:6379"}),
		client2,
		func(context.Context) error { return nil },
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

	if calls := client1.callOrder(); len(calls) != 1 || calls[0] != 1 {
		t.Fatalf("unexpected first runner calls: %v", calls)
	}
	if calls := client2.callOrder(); len(calls) != 0 {
		t.Fatalf("expected second runner to skip execution, got calls %v", calls)
	}
}

func newTestWorkerPlanSchedulerConfig(orgIDs ...int64) *workerconfig.PlanSchedulerConfig {
	if len(orgIDs) == 0 {
		orgIDs = []int64{1}
	}
	return &workerconfig.PlanSchedulerConfig{
		Enable:       true,
		OrgIDs:       orgIDs,
		InitialDelay: 0,
		Interval:     time.Minute,
		LockKey:      "qs:plan-scheduler:test",
		LockTTL:      30 * time.Second,
	}
}
