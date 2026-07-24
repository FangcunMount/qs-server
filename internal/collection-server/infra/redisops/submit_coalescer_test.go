package redisops

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/locklease"
	locksubsystem "github.com/FangcunMount/qs-server/internal/pkg/resilience/locklease/subsystem"
	"github.com/alicebob/miniredis/v2"
	redis "github.com/redis/go-redis/v9"
)

func TestSubmitCoalescerCollapsesOneHundredRequestsAcrossInstances(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	opsHandle := &redisruntime.Handle{
		Family:  redisruntime.FamilyOps,
		Client:  client,
		Builder: keyspace.NewBuilderWithNamespace("ops:runtime"),
	}
	lockHandle := &redisruntime.Handle{
		Family:     redisruntime.FamilyLock,
		Client:     client,
		Builder:    keyspace.NewBuilderWithNamespace("cache:lock"),
		Configured: true,
		Available:  true,
	}
	observer := newCoalescerObserver(100)
	cfg := SubmitCoalescerConfig{
		WaitTimeout:  time.Second,
		PollInterval: 5 * time.Millisecond,
		SignalTTL:    time.Minute,
	}
	instanceA := NewSubmitCoalescerWithObserver(
		opsHandle,
		locksubsystem.New(locksubsystem.Options{Component: "collection-server", Handle: lockHandle}),
		cfg,
		observer,
	)
	instanceB := NewSubmitCoalescerWithObserver(
		opsHandle,
		locksubsystem.New(locksubsystem.Options{Component: "collection-server", Handle: lockHandle}),
		cfg,
		observer,
	)

	ownerStarted := make(chan struct{})
	releaseOwner := make(chan struct{})
	firstResult := make(chan coalescerCallResult, 1)
	var ownerCalls atomic.Int32
	var readbackCalls atomic.Int32
	go func() {
		value, err := instanceA.Run(
			context.Background(),
			"11:submit-1234",
			func(context.Context) (string, error) {
				ownerCalls.Add(1)
				close(ownerStarted)
				<-releaseOwner
				return "42", nil
			},
			func(context.Context) (string, error) {
				readbackCalls.Add(1)
				return "42", nil
			},
		)
		firstResult <- coalescerCallResult{value: value, err: err}
	}()
	<-ownerStarted

	const contenders = 99
	results := make(chan coalescerCallResult, contenders)
	for i := 0; i < contenders; i++ {
		instance := instanceA
		if i%2 == 1 {
			instance = instanceB
		}
		go func() {
			value, err := instance.Run(
				context.Background(),
				"11:submit-1234",
				func(context.Context) (string, error) {
					ownerCalls.Add(1)
					return "unexpected-owner", nil
				},
				func(context.Context) (string, error) {
					readbackCalls.Add(1)
					return "42", nil
				},
			)
			results <- coalescerCallResult{value: value, err: err}
		}()
	}

	observer.waitFor(t, resilience.OutcomeLockContention, contenders)
	close(releaseOwner)

	first := <-firstResult
	if first.err != nil || first.value != "42" {
		t.Fatalf("owner result = (%q, %v), want (42, nil)", first.value, first.err)
	}
	for i := 0; i < contenders; i++ {
		result := <-results
		if result.err != nil || result.value != "42" {
			t.Fatalf("contender result %d = (%q, %v), want (42, nil)", i, result.value, result.err)
		}
	}
	if got := ownerCalls.Load(); got != 1 {
		t.Fatalf("durable owner calls = %d, want 1", got)
	}
	if got := readbackCalls.Load(); got != contenders {
		t.Fatalf("durable readback calls = %d, want %d", got, contenders)
	}
}

func TestSubmitCoalescerContentionWaitsForSignalThenReadsDurableResult(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	opsHandle := &redisruntime.Handle{
		Family:  redisruntime.FamilyOps,
		Client:  client,
		Builder: keyspace.NewBuilderWithNamespace("ops:runtime"),
	}
	lockHandle := &redisruntime.Handle{
		Family:     redisruntime.FamilyLock,
		Client:     client,
		Builder:    keyspace.NewBuilderWithNamespace("cache:lock"),
		Configured: true,
		Available:  true,
	}
	observer := newCoalescerObserver(4)
	cfg := SubmitCoalescerConfig{WaitTimeout: time.Second, PollInterval: 5 * time.Millisecond, SignalTTL: time.Minute}
	instanceA := NewSubmitCoalescerWithObserver(
		opsHandle,
		locksubsystem.New(locksubsystem.Options{Component: "collection-server", Handle: lockHandle}),
		cfg,
		observer,
	)
	instanceB := NewSubmitCoalescerWithObserver(
		opsHandle,
		locksubsystem.New(locksubsystem.Options{Component: "collection-server", Handle: lockHandle}),
		cfg,
		observer,
	)

	ownerStarted := make(chan struct{})
	releaseOwner := make(chan struct{})
	ownerDone := make(chan coalescerCallResult, 1)
	go func() {
		value, err := instanceA.Run(context.Background(), "11:submit-5678", func(context.Context) (string, error) {
			close(ownerStarted)
			<-releaseOwner
			return "84", nil
		}, func(context.Context) (string, error) {
			t.Error("owner must not use the contender readback path")
			return "", nil
		})
		ownerDone <- coalescerCallResult{value: value, err: err}
	}()
	<-ownerStarted

	contenderDone := make(chan coalescerCallResult, 1)
	var contenderOwnerCalls atomic.Int32
	var contenderReadbackCalls atomic.Int32
	go func() {
		value, err := instanceB.Run(context.Background(), "11:submit-5678", func(context.Context) (string, error) {
			contenderOwnerCalls.Add(1)
			return "unexpected-owner", nil
		}, func(context.Context) (string, error) {
			contenderReadbackCalls.Add(1)
			return "84", nil
		})
		contenderDone <- coalescerCallResult{value: value, err: err}
	}()

	observer.waitFor(t, resilience.OutcomeLockContention, 1)
	select {
	case result := <-contenderDone:
		t.Fatalf("contender returned before owner completion: %+v", result)
	default:
	}
	close(releaseOwner)

	if result := <-ownerDone; result.err != nil || result.value != "84" {
		t.Fatalf("owner result = (%q, %v), want (84, nil)", result.value, result.err)
	}
	if result := <-contenderDone; result.err != nil || result.value != "84" {
		t.Fatalf("contender result = (%q, %v), want (84, nil)", result.value, result.err)
	}
	if contenderOwnerCalls.Load() != 0 || contenderReadbackCalls.Load() != 1 {
		t.Fatalf("contender owner/readback calls = %d/%d, want 0/1", contenderOwnerCalls.Load(), contenderReadbackCalls.Load())
	}
}

func TestSubmitCoalescerRedisAcquireFailureDegradesToDurableSubmit(t *testing.T) {
	want := errors.New("redis unavailable")
	coalescer := NewSubmitCoalescerWithObserver(nil, runnerFunc(func(
		context.Context,
		locklease.WorkloadID,
		string,
		time.Duration,
		func(context.Context) error,
	) (locklease.RunResult, error) {
		return locklease.RunResult{}, errors.Join(locklease.ErrLeaseAcquireFailed, want)
	}), DefaultSubmitCoalescerConfig(), newCoalescerObserver(1))

	var ownerCalls atomic.Int32
	value, err := coalescer.Run(t.Context(), "11:submit-redis-down", func(context.Context) (string, error) {
		ownerCalls.Add(1)
		return "42", nil
	}, func(context.Context) (string, error) {
		t.Fatal("Redis acquire failure must use the normal durable submit path")
		return "", nil
	})
	if err != nil || value != "42" || ownerCalls.Load() != 1 {
		t.Fatalf("Run() = value=%q owner_calls=%d err=%v", value, ownerCalls.Load(), err)
	}
}

func TestSubmitCoalescerCanceledWhileWaitingDoesNotReadBack(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	observer := newCoalescerObserver(2)
	coalescer := NewSubmitCoalescerWithObserver(
		&redisruntime.Handle{Client: client, Builder: keyspace.NewBuilderWithNamespace("ops:runtime")},
		contentionRunner{},
		SubmitCoalescerConfig{WaitTimeout: time.Second, PollInterval: 5 * time.Millisecond, SignalTTL: time.Minute},
		observer,
	)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	var readbackCalls atomic.Int32
	go func() {
		_, err := coalescer.Run(ctx, "11:submit-canceled", func(context.Context) (string, error) {
			t.Error("contender must not execute owner path")
			return "", nil
		}, func(context.Context) (string, error) {
			readbackCalls.Add(1)
			return "42", nil
		})
		done <- err
	}()
	observer.waitFor(t, resilience.OutcomeLockContention, 1)
	cancel()

	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context.Canceled", err)
	}
	if readbackCalls.Load() != 0 {
		t.Fatalf("readback calls = %d, want 0 after cancellation", readbackCalls.Load())
	}
}

func TestSubmitCoalescerStaleLeaseFallsBackAfterBoundedWait(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	coalescer := NewSubmitCoalescerWithObserver(
		&redisruntime.Handle{Client: client, Builder: keyspace.NewBuilderWithNamespace("ops:runtime")},
		contentionRunner{},
		SubmitCoalescerConfig{WaitTimeout: 20 * time.Millisecond, PollInterval: 5 * time.Millisecond, SignalTTL: time.Minute},
		newCoalescerObserver(2),
	)

	started := time.Now()
	var readbackCalls atomic.Int32
	value, err := coalescer.Run(t.Context(), "11:submit-stale", func(context.Context) (string, error) {
		t.Fatal("stale lease contender must not execute owner path before bounded wait")
		return "", nil
	}, func(context.Context) (string, error) {
		readbackCalls.Add(1)
		return "42", nil
	})
	elapsed := time.Since(started)
	if err != nil || value != "42" || readbackCalls.Load() != 1 {
		t.Fatalf("Run() = value=%q readback_calls=%d err=%v", value, readbackCalls.Load(), err)
	}
	if elapsed < 15*time.Millisecond || elapsed > 250*time.Millisecond {
		t.Fatalf("bounded wait elapsed = %v, want approximately 20ms", elapsed)
	}
}

func TestSubmitCoalescerSignalNeverOverridesDurableConflict(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	coalescer := NewSubmitCoalescerWithObserver(
		&redisruntime.Handle{Client: client, Builder: keyspace.NewBuilderWithNamespace("ops:runtime")},
		contentionRunner{},
		DefaultSubmitCoalescerConfig(),
		newCoalescerObserver(2),
	)
	if err := client.Set(t.Context(), coalescer.completionSignalKey("11:submit-conflict"), "untrusted-value", time.Minute).Err(); err != nil {
		t.Fatalf("seed completion signal: %v", err)
	}

	want := errors.New("durable fingerprint conflict")
	var readbackCalls atomic.Int32
	value, err := coalescer.Run(t.Context(), "11:submit-conflict", func(context.Context) (string, error) {
		t.Fatal("contender must not execute owner path")
		return "", nil
	}, func(context.Context) (string, error) {
		readbackCalls.Add(1)
		return "", want
	})
	if value != "" || !errors.Is(err, want) || readbackCalls.Load() != 1 {
		t.Fatalf("Run() = value=%q readback_calls=%d err=%v, want durable conflict", value, readbackCalls.Load(), err)
	}
}

func TestSubmitCoalescerSignalAndReleaseFailuresCannotOverrideDurableSuccess(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	if err := client.Close(); err != nil {
		t.Fatalf("close Redis client: %v", err)
	}
	releaseErr := errors.New("release failed")
	coalescer := NewSubmitCoalescerWithObserver(
		&redisruntime.Handle{Client: client, Builder: keyspace.NewBuilderWithNamespace("ops:runtime")},
		runnerFunc(func(
			ctx context.Context,
			_ locklease.WorkloadID,
			_ string,
			_ time.Duration,
			body func(context.Context) error,
		) (locklease.RunResult, error) {
			return locklease.RunResult{Acquired: true, ReleaseErr: releaseErr}, body(ctx)
		}),
		DefaultSubmitCoalescerConfig(),
		newCoalescerObserver(2),
	)

	value, err := coalescer.Run(t.Context(), "11:submit-release-error", func(context.Context) (string, error) {
		return "42", nil
	}, func(context.Context) (string, error) {
		t.Fatal("owner must not read back")
		return "", nil
	})
	if err != nil || value != "42" {
		t.Fatalf("Run() = value=%q err=%v, want durable owner success", value, err)
	}
}

type coalescerCallResult struct {
	value string
	err   error
}

type runnerFunc func(
	context.Context,
	locklease.WorkloadID,
	string,
	time.Duration,
	func(context.Context) error,
) (locklease.RunResult, error)

func (f runnerFunc) Run(
	ctx context.Context,
	workload locklease.WorkloadID,
	key string,
	ttl time.Duration,
	body func(context.Context) error,
) (locklease.RunResult, error) {
	return f(ctx, workload, key, ttl, body)
}

type contentionRunner struct{}

func (contentionRunner) Run(
	context.Context,
	locklease.WorkloadID,
	string,
	time.Duration,
	func(context.Context) error,
) (locklease.RunResult, error) {
	return locklease.RunResult{}, nil
}

type coalescerObserver struct {
	mu       sync.Mutex
	counts   map[resilience.Outcome]int
	observed chan resilience.Outcome
}

func newCoalescerObserver(capacity int) *coalescerObserver {
	return &coalescerObserver{
		counts:   make(map[resilience.Outcome]int),
		observed: make(chan resilience.Outcome, capacity),
	}
}

func (o *coalescerObserver) ObserveDecision(_ context.Context, decision resilience.Decision) {
	o.mu.Lock()
	o.counts[decision.Outcome]++
	o.mu.Unlock()
	select {
	case o.observed <- decision.Outcome:
	default:
	}
}

func (o *coalescerObserver) waitFor(t *testing.T, outcome resilience.Outcome, count int) {
	t.Helper()
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	for {
		o.mu.Lock()
		got := o.counts[outcome]
		o.mu.Unlock()
		if got >= count {
			return
		}
		select {
		case <-o.observed:
		case <-deadline.C:
			t.Fatalf("observed %s %d times, want at least %d", outcome, got, count)
		}
	}
}
