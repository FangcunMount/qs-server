package cache

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type readThroughValue struct {
	Value string
}

func TestReadThroughUsesPolicyScopedSingleflight(t *testing.T) {
	t.Parallel()

	var loadCount atomic.Int32
	release := make(chan struct{})
	started := make(chan struct{}, 1)

	policy := CachePolicy{
		Singleflight: PolicySwitchEnabled,
	}

	run := func() (*readThroughValue, error) {
		return ReadThrough(context.Background(), ReadThroughOptions[readThroughValue]{
			PolicyKey: PolicyAssessmentDetail,
			CacheKey:  "assessment:detail:42",
			Policy:    policy,
			GetCached: func(context.Context) (*readThroughValue, error) {
				return nil, ErrCacheNotFound
			},
			Load: func(context.Context) (*readThroughValue, error) {
				loadCount.Add(1)
				select {
				case started <- struct{}{}:
				default:
				}
				<-release
				return &readThroughValue{Value: "ok"}, nil
			},
		})
	}

	var wg sync.WaitGroup
	results := make(chan *readThroughValue, 2)
	errors := make(chan error, 2)
	begin := make(chan struct{})
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-begin
			value, err := run()
			results <- value
			errors <- err
		}()
	}
	close(begin)

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("loader was not invoked")
	}
	close(release)
	wg.Wait()
	close(results)
	close(errors)

	if got := loadCount.Load(); got != 1 {
		t.Fatalf("expected singleflight loader to run once, got %d", got)
	}
	for err := range errors {
		if err != nil {
			t.Fatalf("unexpected read-through error: %v", err)
		}
	}
	for value := range results {
		if value == nil || value.Value != "ok" {
			t.Fatalf("unexpected read-through value: %#v", value)
		}
	}
}

func TestReadThroughSingleflightIsScopedByPolicyKey(t *testing.T) {
	t.Parallel()

	var loadCount atomic.Int32
	release := make(chan struct{})
	started := make(chan struct{}, 2)

	policy := CachePolicy{
		Singleflight: PolicySwitchEnabled,
	}

	run := func(policyKey CachePolicyKey) {
		t.Helper()
		_, err := ReadThrough(context.Background(), ReadThroughOptions[readThroughValue]{
			PolicyKey: policyKey,
			CacheKey:  "shared:key",
			Policy:    policy,
			GetCached: func(context.Context) (*readThroughValue, error) {
				return nil, ErrCacheNotFound
			},
			Load: func(context.Context) (*readThroughValue, error) {
				loadCount.Add(1)
				started <- struct{}{}
				<-release
				return &readThroughValue{Value: string(policyKey)}, nil
			},
		})
		if err != nil {
			t.Errorf("unexpected read-through error for %s: %v", policyKey, err)
		}
	}

	var wg sync.WaitGroup
	begin := make(chan struct{})
	for _, policyKey := range []CachePolicyKey{PolicyAssessmentDetail, PolicyPlan} {
		wg.Add(1)
		go func(policyKey CachePolicyKey) {
			defer wg.Done()
			<-begin
			run(policyKey)
		}(policyKey)
	}
	close(begin)

	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-time.After(2 * time.Second):
			t.Fatal("expected both policy-scoped loaders to start")
		}
	}
	close(release)
	wg.Wait()

	if got := loadCount.Load(); got != 2 {
		t.Fatalf("expected per-policy singleflight groups to isolate loaders, got %d", got)
	}
}

func TestReadThroughDegradesCacheReadErrorToMiss(t *testing.T) {
	t.Parallel()

	var loadCount atomic.Int32
	value, err := ReadThrough(context.Background(), ReadThroughOptions[readThroughValue]{
		PolicyKey: PolicyAssessmentDetail,
		CacheKey:  "assessment:detail:42",
		Policy:    CachePolicy{},
		GetCached: func(context.Context) (*readThroughValue, error) {
			return nil, errors.New("redis unavailable")
		},
		Load: func(context.Context) (*readThroughValue, error) {
			loadCount.Add(1)
			return &readThroughValue{Value: "loaded"}, nil
		},
	})
	if err != nil {
		t.Fatalf("ReadThrough() error = %v", err)
	}
	if value == nil || value.Value != "loaded" {
		t.Fatalf("ReadThrough() value = %#v, want loaded", value)
	}
	if got := loadCount.Load(); got != 1 {
		t.Fatalf("loader count = %d, want 1", got)
	}
}
