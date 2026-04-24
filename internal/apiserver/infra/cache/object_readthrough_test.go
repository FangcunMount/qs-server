package cache

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
)

func TestReadThroughObjectLoadsWritesPositiveAndNegativeEntries(t *testing.T) {
	t.Parallel()

	store, _, cleanup := newObjectCacheContractStore(t, cachepolicy.CachePolicy{
		Negative:    cachepolicy.PolicySwitchEnabled,
		NegativeTTL: time.Minute,
	})
	defer cleanup()

	ctx := context.Background()
	var loadCalls int
	got, err := ReadThroughObject(ctx, ObjectReadThroughOptions[objectCacheContractValue]{
		PolicyKey: cachepolicy.PolicyAssessmentDetail,
		CacheKey:  "object:readthrough:positive",
		Policy:    cachepolicy.CachePolicy{},
		Store:     store,
		Load: func(context.Context) (*objectCacheContractValue, error) {
			loadCalls++
			return &objectCacheContractValue{ID: 100, Name: "loaded-by-helper"}, nil
		},
	})
	if err != nil {
		t.Fatalf("ReadThroughObject() positive error = %v", err)
	}
	if got == nil || got.Name != "loaded-by-helper" {
		t.Fatalf("ReadThroughObject() positive value = %#v, want loaded-by-helper", got)
	}
	if loadCalls != 1 {
		t.Fatalf("load calls = %d, want 1", loadCalls)
	}
	cached, err := store.Get(ctx, "object:readthrough:positive")
	if err != nil {
		t.Fatalf("Get() positive cache error = %v", err)
	}
	if cached == nil || cached.Name != "loaded-by-helper" {
		t.Fatalf("cached positive value = %#v, want loaded-by-helper", cached)
	}

	got, err = ReadThroughObject(ctx, ObjectReadThroughOptions[objectCacheContractValue]{
		PolicyKey: cachepolicy.PolicyAssessmentDetail,
		CacheKey:  "object:readthrough:negative",
		Policy: cachepolicy.CachePolicy{
			Negative: cachepolicy.PolicySwitchEnabled,
		},
		Store:         store,
		CacheNegative: true,
		Load: func(context.Context) (*objectCacheContractValue, error) {
			return nil, nil
		},
	})
	if err != nil {
		t.Fatalf("ReadThroughObject() negative error = %v", err)
	}
	if got != nil {
		t.Fatalf("ReadThroughObject() negative value = %#v, want nil", got)
	}
	negative, err := store.Get(ctx, "object:readthrough:negative")
	if err != nil {
		t.Fatalf("Get() negative cache error = %v", err)
	}
	if negative != nil {
		t.Fatalf("cached negative value = %#v, want nil", negative)
	}
}

func TestReadThroughObjectAsyncWriteback(t *testing.T) {
	t.Parallel()

	store, _, cleanup := newObjectCacheContractStore(t, cachepolicy.CachePolicy{
		Negative:    cachepolicy.PolicySwitchEnabled,
		NegativeTTL: time.Minute,
	})
	defer cleanup()

	ctx := context.Background()
	got, err := ReadThroughObject(ctx, ObjectReadThroughOptions[objectCacheContractValue]{
		PolicyKey: cachepolicy.PolicyAssessmentDetail,
		CacheKey:  "object:readthrough:async-positive",
		Policy:    cachepolicy.CachePolicy{},
		Store:     store,
		Load: func(context.Context) (*objectCacheContractValue, error) {
			return &objectCacheContractValue{ID: 101, Name: "async-helper"}, nil
		},
		AsyncSetCached: true,
	})
	if err != nil {
		t.Fatalf("ReadThroughObject() async positive error = %v", err)
	}
	if got == nil || got.Name != "async-helper" {
		t.Fatalf("ReadThroughObject() async positive value = %#v, want async-helper", got)
	}
	waitFor(t, func() bool {
		exists, _ := store.Exists(ctx, "object:readthrough:async-positive")
		return exists
	})

	got, err = ReadThroughObject(ctx, ObjectReadThroughOptions[objectCacheContractValue]{
		PolicyKey: cachepolicy.PolicyAssessmentDetail,
		CacheKey:  "object:readthrough:async-negative",
		Policy: cachepolicy.CachePolicy{
			Negative: cachepolicy.PolicySwitchEnabled,
		},
		Store:            store,
		Load:             func(context.Context) (*objectCacheContractValue, error) { return nil, nil },
		CacheNegative:    true,
		AsyncSetNegative: true,
	})
	if err != nil {
		t.Fatalf("ReadThroughObject() async negative error = %v", err)
	}
	if got != nil {
		t.Fatalf("ReadThroughObject() async negative value = %#v, want nil", got)
	}
	waitFor(t, func() bool {
		exists, _ := store.Exists(ctx, "object:readthrough:async-negative")
		return exists
	})
}

func TestReadThroughObjectForwardsInjectedRunner(t *testing.T) {
	t.Parallel()

	var loadCount atomic.Int32
	release := make(chan struct{})
	started := make(chan struct{}, 2)
	policy := cachepolicy.CachePolicy{
		Singleflight: cachepolicy.PolicySwitchEnabled,
	}

	run := func(runner *ReadThroughRunner[objectCacheContractValue]) error {
		_, err := ReadThroughObject(context.Background(), ObjectReadThroughOptions[objectCacheContractValue]{
			PolicyKey: cachepolicy.PolicyAssessmentDetail,
			CacheKey:  "same:object:readthrough:key",
			Policy:    policy,
			Runner:    runner,
			Load: func(context.Context) (*objectCacheContractValue, error) {
				loadCount.Add(1)
				started <- struct{}{}
				<-release
				return &objectCacheContractValue{ID: 102, Name: "isolated-object"}, nil
			},
		})
		return err
	}

	var wg sync.WaitGroup
	errors := make(chan error, 2)
	begin := make(chan struct{})
	for _, runner := range []*ReadThroughRunner[objectCacheContractValue]{
		NewReadThroughRunner[objectCacheContractValue](NewSingleflightCoordinator()),
		NewReadThroughRunner[objectCacheContractValue](NewSingleflightCoordinator()),
	} {
		wg.Add(1)
		go func(runner *ReadThroughRunner[objectCacheContractValue]) {
			defer wg.Done()
			<-begin
			errors <- run(runner)
		}(runner)
	}
	close(begin)

	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-time.After(2 * time.Second):
			t.Fatal("expected both object read-through loaders to start")
		}
	}
	close(release)
	wg.Wait()
	close(errors)

	if got := loadCount.Load(); got != 2 {
		t.Fatalf("expected injected runners to isolate object loaders, got %d", got)
	}
	for err := range errors {
		if err != nil {
			t.Fatalf("unexpected object read-through error: %v", err)
		}
	}
}
