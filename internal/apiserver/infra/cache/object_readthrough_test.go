package cache

import (
	"context"
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
