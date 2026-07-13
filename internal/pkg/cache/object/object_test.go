package object

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	sharedcache "github.com/FangcunMount/qs-server/internal/pkg/cache"
	redisstore "github.com/FangcunMount/qs-server/internal/pkg/cache/redis"
	"github.com/FangcunMount/qs-server/internal/pkg/loadguard"
	"github.com/alicebob/miniredis/v2"
	redis "github.com/redis/go-redis/v9"
)

type contractValue struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

var contractCodec = Codec[contractValue]{
	EncodeFunc: func(value *contractValue) ([]byte, error) { return json.Marshal(value) },
	DecodeFunc: func(data []byte) (*contractValue, error) {
		var value contractValue
		if err := json.Unmarshal(data, &value); err != nil {
			return nil, err
		}
		return &value, nil
	},
}

func TestStorePreservesPayloadNegativeAndDeleteContracts(t *testing.T) {
	store, _, cleanup := newContractStore(t, sharedcache.Policy{
		Compress:    sharedcache.PolicySwitchEnabled,
		NegativeTTL: time.Minute,
	})
	defer cleanup()
	ctx := context.Background()

	want := &contractValue{ID: 42, Name: "cached"}
	policy := sharedcache.Policy{TTL: time.Minute, NegativeTTL: time.Minute, Compress: sharedcache.PolicySwitchEnabled}
	if err := store.Set(ctx, "object:42", want, policy); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := store.Get(ctx, "object:42")
	if err != nil || got == nil || *got != *want {
		t.Fatalf("Get = %#v, %v", got, err)
	}
	if err := store.SetNegative(ctx, "object:missing", policy); err != nil {
		t.Fatalf("SetNegative: %v", err)
	}
	if got, err := store.Get(ctx, "object:missing"); err != nil || got != nil {
		t.Fatalf("negative Get = %#v, %v", got, err)
	}
	if err := store.Delete(ctx, "object:42"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := store.Get(ctx, "object:42"); !errors.Is(err, sharedcache.ErrMiss) {
		t.Fatalf("Get after delete = %v, want ErrMiss", err)
	}
}

func TestReadThroughCoalescesMissAndWritesCache(t *testing.T) {
	policy := sharedcache.Policy{Singleflight: sharedcache.PolicySwitchEnabled}
	store, _, cleanup := newContractStore(t, policy)
	policies := sharedcache.NewRegistry(sharedcache.EffectiveCapability{Capability: "assessment_detail", Policy: policy})
	defer cleanup()
	var loads atomic.Int32
	load := func(context.Context) (*contractValue, error) {
		loads.Add(1)
		time.Sleep(10 * time.Millisecond)
		return &contractValue{ID: 7, Name: "loaded"}, nil
	}

	errCh := make(chan error, 8)
	for i := 0; i < 8; i++ {
		go func() {
			_, err := ReadThrough(context.Background(), ReadThroughOptions[contractValue]{
				Capability:     "assessment_detail",
				CacheKey:       "object:miss",
				PolicyProvider: policies,
				Store:          store,
				Load:           load,
			})
			errCh <- err
		}()
	}
	for i := 0; i < 8; i++ {
		if err := <-errCh; err != nil {
			t.Fatalf("ReadThrough: %v", err)
		}
	}
	if loads.Load() != 1 {
		t.Fatalf("loads = %d, want 1", loads.Load())
	}
	if got, err := store.Get(context.Background(), "object:miss"); err != nil || got == nil || got.Name != "loaded" {
		t.Fatalf("cached = %#v, %v", got, err)
	}
}

func TestReadThroughFailsOpenOnStoreError(t *testing.T) {
	boom := errors.New("redis unavailable")
	store := NewStore(StoreOptions[contractValue]{
		Store: errorStore{err: boom},
		Codec: contractCodec,
	})
	got, err := ReadThrough(context.Background(), ReadThroughOptions[contractValue]{
		Capability: "assessment_detail",
		CacheKey:   "object:error",
		Store:      store,
		Load: func(context.Context) (*contractValue, error) {
			return &contractValue{ID: 1, Name: "fallback"}, nil
		},
	})
	if err != nil || got == nil || got.Name != "fallback" {
		t.Fatalf("ReadThrough = %#v, %v", got, err)
	}
}

func newContractStore(t *testing.T, policy sharedcache.Policy) (*Store[contractValue], redis.UniversalClient, func()) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store := NewStore(StoreOptions[contractValue]{
		Store: redisstore.NewStore(client), Codec: contractCodec,
		Coalescer: loadguard.NewCoalescer(true),
	})
	return store, client, func() { _ = client.Close(); mr.Close() }
}

type errorStore struct{ err error }

func (s errorStore) Get(context.Context, string) ([]byte, error)              { return nil, s.err }
func (s errorStore) Set(context.Context, string, []byte, time.Duration) error { return s.err }
func (s errorStore) Delete(context.Context, string) error                     { return s.err }
func (s errorStore) Exists(context.Context, string) (bool, error)             { return false, s.err }
