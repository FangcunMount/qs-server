package cachehotset

import (
	"context"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/cachetarget"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheobservability"
	"github.com/FangcunMount/qs-server/internal/pkg/rediskey"
	"github.com/FangcunMount/qs-server/internal/pkg/redisplane"
	"github.com/alicebob/miniredis/v2"
	redis "github.com/redis/go-redis/v9"
)

type testFamilyObserver struct {
	component string
}

func (o testFamilyObserver) ObserveFamilySuccess(family string) {
	cacheobservability.ObserveFamilySuccess(o.component, family)
}

func (o testFamilyObserver) ObserveFamilyFailure(family string, err error) {
	cacheobservability.ObserveFamilyFailure(o.component, family, err)
}

func TestRedisStoreRecordAndTopWithScores(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	recorder := NewRedisStore(
		client,
		rediskey.NewBuilderWithNamespace("prod:cache:meta"),
		Options{Enable: true, TopN: 10, MaxItemsPerKind: 20},
	)
	store, ok := recorder.(*RedisStore)
	if !ok {
		t.Fatalf("recorder type = %T, want *RedisStore", recorder)
	}
	target := cachetarget.WarmupTarget{
		Family: redisplane.FamilyStatic,
		Kind:   cachetarget.WarmupKindStaticScaleList,
		Scope:  "published",
	}
	if err := store.Record(context.Background(), target); err != nil {
		t.Fatalf("Record() error = %v", err)
	}
	items, err := store.TopWithScores(context.Background(), redisplane.FamilyStatic, cachetarget.WarmupKindStaticScaleList, 10)
	if err != nil {
		t.Fatalf("TopWithScores() error = %v", err)
	}
	if len(items) != 1 || items[0].Target != target || items[0].Score != 1 {
		t.Fatalf("items = %#v, want recorded target with score 1", items)
	}
}

func TestRedisStoreNilDisabledNoOpAndSuppression(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	if got := NewRedisStore(client, rediskey.NewBuilderWithNamespace("prod:cache:meta"), Options{}); got != nil {
		t.Fatalf("disabled store = %T, want nil", got)
	}

	recorder := NewRedisStore(
		client,
		rediskey.NewBuilderWithNamespace("prod:cache:meta"),
		Options{Enable: true},
	)
	store := recorder.(*RedisStore)
	target := cachetarget.WarmupTarget{
		Family: redisplane.FamilyStatic,
		Kind:   cachetarget.WarmupKindStaticScaleList,
		Scope:  "published",
	}
	if err := store.Record(cachetarget.SuppressHotsetRecording(context.Background()), target); err != nil {
		t.Fatalf("Record() suppressed error = %v", err)
	}
	items, err := store.TopWithScores(context.Background(), redisplane.FamilyStatic, cachetarget.WarmupKindStaticScaleList, 10)
	if err != nil {
		t.Fatalf("TopWithScores() error = %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("items after suppressed record = %#v, want empty", items)
	}
}

func TestRedisStoreObserverUsesInjectedComponent(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	registry := cacheobservability.NewFamilyStatusRegistry("hotset-observer")
	registry.Update(cacheobservability.FamilyStatus{
		Component: "hotset-observer",
		Family:    string(redisplane.FamilyMeta),
		Available: false,
		Degraded:  true,
		Mode:      cacheobservability.FamilyModeDegraded,
	})

	recorder := NewRedisStoreWithObserver(
		client,
		rediskey.NewBuilderWithNamespace("prod:cache:meta"),
		Options{Enable: true, TopN: 10, MaxItemsPerKind: 20},
		testFamilyObserver{component: "hotset-observer"},
	)
	if recorder == nil {
		t.Fatal("recorder = nil, want enabled hotset recorder")
	}
	if err := recorder.Record(context.Background(), cachetarget.WarmupTarget{
		Family: redisplane.FamilyStatic,
		Kind:   cachetarget.WarmupKindStaticScaleList,
		Scope:  "SDS",
	}); err != nil {
		t.Fatalf("Record() error = %v", err)
	}

	snapshot := cacheobservability.SnapshotForComponent("hotset-observer", registry)
	if !snapshot.Summary.Ready {
		t.Fatalf("runtime summary ready = false, want true after observed success: %#v", snapshot.Summary)
	}
}
