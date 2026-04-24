package cache

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	"github.com/alicebob/miniredis/v2"
	redis "github.com/redis/go-redis/v9"
)

type objectCacheContractValue struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

var objectCacheContractCodec = CacheEntryCodec[objectCacheContractValue]{
	EncodeFunc: func(value *objectCacheContractValue) ([]byte, error) {
		return json.Marshal(value)
	},
	DecodeFunc: func(data []byte) (*objectCacheContractValue, error) {
		var value objectCacheContractValue
		if err := json.Unmarshal(data, &value); err != nil {
			return nil, err
		}
		return &value, nil
	},
}

func TestObjectCacheStoreGetDecodesCompressedPositiveHit(t *testing.T) {
	t.Parallel()

	store, client, cleanup := newObjectCacheContractStore(t, cachepolicy.CachePolicy{
		Compress: cachepolicy.PolicySwitchEnabled,
	})
	defer cleanup()

	ctx := context.Background()
	want := &objectCacheContractValue{ID: 42, Name: "cached"}
	if err := store.Set(ctx, "object:42", want); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	rawJSON, err := objectCacheContractCodec.Encode(want)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	payload, err := NewRedisCache(client).Get(ctx, "object:42")
	if err != nil {
		t.Fatalf("redis Get() error = %v", err)
	}
	if string(payload) == string(rawJSON) {
		t.Fatal("stored payload should be compressed, got raw JSON")
	}

	got, err := store.Get(ctx, "object:42")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got == nil || got.ID != want.ID || got.Name != want.Name {
		t.Fatalf("Get() value = %#v, want %#v", got, want)
	}
}

func TestObjectCacheStoreReadThroughMissLoadsAndWritesPositiveCache(t *testing.T) {
	t.Parallel()

	store, _, cleanup := newObjectCacheContractStore(t, cachepolicy.CachePolicy{})
	defer cleanup()

	ctx := context.Background()
	var loadCalls int
	got, err := ReadThrough(ctx, ReadThroughOptions[objectCacheContractValue]{
		PolicyKey: cachepolicy.PolicyAssessmentDetail,
		CacheKey:  "object:miss",
		Policy:    cachepolicy.CachePolicy{},
		GetCached: func(ctx context.Context) (*objectCacheContractValue, error) {
			return store.Get(ctx, "object:miss")
		},
		Load: func(context.Context) (*objectCacheContractValue, error) {
			loadCalls++
			return &objectCacheContractValue{ID: 7, Name: "loaded"}, nil
		},
		SetCached: func(ctx context.Context, value *objectCacheContractValue) error {
			return store.Set(ctx, "object:miss", value)
		},
	})
	if err != nil {
		t.Fatalf("ReadThrough() error = %v", err)
	}
	if got == nil || got.Name != "loaded" {
		t.Fatalf("ReadThrough() value = %#v, want loaded", got)
	}
	if loadCalls != 1 {
		t.Fatalf("load calls = %d, want 1", loadCalls)
	}

	cached, err := store.Get(ctx, "object:miss")
	if err != nil {
		t.Fatalf("Get() after read-through error = %v", err)
	}
	if cached == nil || cached.Name != "loaded" {
		t.Fatalf("cached value = %#v, want loaded", cached)
	}
}

func TestObjectCacheStoreNegativeEntryReturnsNilValue(t *testing.T) {
	t.Parallel()

	store, _, cleanup := newObjectCacheContractStore(t, cachepolicy.CachePolicy{
		NegativeTTL: time.Minute,
	})
	defer cleanup()

	ctx := context.Background()
	if err := store.SetNegative(ctx, "object:negative"); err != nil {
		t.Fatalf("SetNegative() error = %v", err)
	}

	got, err := store.Get(ctx, "object:negative")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got != nil {
		t.Fatalf("Get() value = %#v, want nil negative sentinel", got)
	}
}

func TestObjectCacheStoreReadThroughDegradesRedisErrorToSourceLoad(t *testing.T) {
	t.Parallel()

	store := NewObjectCacheStore(ObjectCacheStoreOptions[objectCacheContractValue]{
		Cache:       errorCache{err: errors.New("redis unavailable")},
		PolicyKey:   cachepolicy.PolicyAssessmentDetail,
		Policy:      cachepolicy.CachePolicy{},
		TTL:         time.Minute,
		NegativeTTL: time.Minute,
		Codec:       objectCacheContractCodec,
	})

	var loadCalls int
	got, err := ReadThrough(context.Background(), ReadThroughOptions[objectCacheContractValue]{
		PolicyKey: cachepolicy.PolicyAssessmentDetail,
		CacheKey:  "object:error",
		Policy:    cachepolicy.CachePolicy{},
		GetCached: func(ctx context.Context) (*objectCacheContractValue, error) {
			return store.Get(ctx, "object:error")
		},
		Load: func(context.Context) (*objectCacheContractValue, error) {
			loadCalls++
			return &objectCacheContractValue{ID: 1, Name: "fallback"}, nil
		},
	})
	if err != nil {
		t.Fatalf("ReadThrough() error = %v", err)
	}
	if got == nil || got.Name != "fallback" {
		t.Fatalf("ReadThrough() value = %#v, want fallback", got)
	}
	if loadCalls != 1 {
		t.Fatalf("load calls = %d, want 1", loadCalls)
	}
}

func TestObjectCacheStoreDeleteAndNilCacheNoOp(t *testing.T) {
	t.Parallel()

	store, _, cleanup := newObjectCacheContractStore(t, cachepolicy.CachePolicy{})
	defer cleanup()

	ctx := context.Background()
	if err := store.Set(ctx, "object:delete", &objectCacheContractValue{ID: 1, Name: "delete"}); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	exists, err := store.Exists(ctx, "object:delete")
	if err != nil {
		t.Fatalf("Exists() error = %v", err)
	}
	if !exists {
		t.Fatal("cache entry should exist before delete")
	}
	if err := store.Delete(ctx, "object:delete"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if err := store.Delete(ctx, "object:delete"); err != nil {
		t.Fatalf("Delete() repeated error = %v", err)
	}
	exists, err = store.Exists(ctx, "object:delete")
	if err != nil {
		t.Fatalf("Exists() after delete error = %v", err)
	}
	if exists {
		t.Fatal("cache entry should not exist after delete")
	}

	nilStore := NewObjectCacheStore(ObjectCacheStoreOptions[objectCacheContractValue]{
		PolicyKey:   cachepolicy.PolicyAssessmentDetail,
		Policy:      cachepolicy.CachePolicy{},
		TTL:         time.Minute,
		NegativeTTL: time.Minute,
		Codec:       objectCacheContractCodec,
	})
	if _, err := nilStore.Get(ctx, "object:nil"); !errors.Is(err, ErrCacheNotFound) {
		t.Fatalf("nil cache Get() error = %v, want ErrCacheNotFound", err)
	}
	if err := nilStore.Set(ctx, "object:nil", &objectCacheContractValue{}); err != nil {
		t.Fatalf("nil cache Set() error = %v", err)
	}
	if err := nilStore.SetNegative(ctx, "object:nil"); err != nil {
		t.Fatalf("nil cache SetNegative() error = %v", err)
	}
	if err := nilStore.Delete(ctx, "object:nil"); err != nil {
		t.Fatalf("nil cache Delete() error = %v", err)
	}
	exists, err = nilStore.Exists(ctx, "object:nil")
	if err != nil {
		t.Fatalf("nil cache Exists() error = %v", err)
	}
	if exists {
		t.Fatal("nil cache Exists() should return false")
	}
}

func TestObjectCacheStoreReadThroughAsyncWriteback(t *testing.T) {
	t.Parallel()

	store, _, cleanup := newObjectCacheContractStore(t, cachepolicy.CachePolicy{
		Negative:    cachepolicy.PolicySwitchEnabled,
		NegativeTTL: time.Minute,
	})
	defer cleanup()

	ctx := context.Background()
	got, err := ReadThrough(ctx, ReadThroughOptions[objectCacheContractValue]{
		PolicyKey: cachepolicy.PolicyAssessmentDetail,
		CacheKey:  "object:async-positive",
		Policy:    cachepolicy.CachePolicy{},
		GetCached: func(ctx context.Context) (*objectCacheContractValue, error) {
			return store.Get(ctx, "object:async-positive")
		},
		Load: func(context.Context) (*objectCacheContractValue, error) {
			return &objectCacheContractValue{ID: 9, Name: "async"}, nil
		},
		SetCached: func(ctx context.Context, value *objectCacheContractValue) error {
			return store.Set(ctx, "object:async-positive", value)
		},
		AsyncSetCached: true,
	})
	if err != nil {
		t.Fatalf("ReadThrough() positive error = %v", err)
	}
	if got == nil || got.Name != "async" {
		t.Fatalf("ReadThrough() positive value = %#v, want async", got)
	}
	waitFor(t, func() bool {
		exists, _ := store.Exists(ctx, "object:async-positive")
		return exists
	})

	got, err = ReadThrough(ctx, ReadThroughOptions[objectCacheContractValue]{
		PolicyKey: cachepolicy.PolicyAssessmentDetail,
		CacheKey:  "object:async-negative",
		Policy: cachepolicy.CachePolicy{
			Negative: cachepolicy.PolicySwitchEnabled,
		},
		GetCached: func(ctx context.Context) (*objectCacheContractValue, error) {
			return store.Get(ctx, "object:async-negative")
		},
		Load: func(context.Context) (*objectCacheContractValue, error) {
			return nil, nil
		},
		SetNegativeCached: func(ctx context.Context) error {
			return store.SetNegative(ctx, "object:async-negative")
		},
		AsyncSetNegative: true,
	})
	if err != nil {
		t.Fatalf("ReadThrough() negative error = %v", err)
	}
	if got != nil {
		t.Fatalf("ReadThrough() negative value = %#v, want nil", got)
	}
	waitFor(t, func() bool {
		exists, _ := store.Exists(ctx, "object:async-negative")
		return exists
	})
	negative, err := store.Get(ctx, "object:async-negative")
	if err != nil {
		t.Fatalf("Get() async negative error = %v", err)
	}
	if negative != nil {
		t.Fatalf("Get() async negative value = %#v, want nil", negative)
	}
}

func newObjectCacheContractStore(t *testing.T, policy cachepolicy.CachePolicy) (*ObjectCacheStore[objectCacheContractValue], redis.UniversalClient, func()) {
	t.Helper()

	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store := NewObjectCacheStore(ObjectCacheStoreOptions[objectCacheContractValue]{
		Cache:       NewRedisCache(client),
		PolicyKey:   cachepolicy.PolicyAssessmentDetail,
		Policy:      policy,
		TTL:         time.Minute,
		NegativeTTL: time.Minute,
		Codec:       objectCacheContractCodec,
	})
	cleanup := func() {
		_ = client.Close()
		mr.Close()
	}
	return store, client, cleanup
}

type errorCache struct {
	err error
}

func (c errorCache) Get(context.Context, string) ([]byte, error) {
	return nil, c.err
}

func (c errorCache) Set(context.Context, string, []byte, time.Duration) error {
	return c.err
}

func (c errorCache) Delete(context.Context, string) error {
	return c.err
}

func (c errorCache) Exists(context.Context, string) (bool, error) {
	return false, c.err
}
