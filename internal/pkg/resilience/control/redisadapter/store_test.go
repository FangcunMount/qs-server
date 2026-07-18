package redisadapter

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/control"
	"github.com/alicebob/miniredis/v2"
	redis "github.com/redis/go-redis/v9"
)

func TestStoreCompareAndSwapAndDelete(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	store := NewStore(client, keyspace.NewBuilderWithNamespace("ops:runtime"))

	published, err := store.CompareAndSwap(context.Background(), "rate:apiserver:query", 0, control.VersionedState{Payload: []byte(`{"qps":10}`)}, time.Minute)
	if err != nil || published.Version != 1 {
		t.Fatalf("CompareAndSwap() = %+v, %v", published, err)
	}
	if _, err := store.CompareAndSwap(context.Background(), "rate:apiserver:query", 0, control.VersionedState{}, time.Minute); !errors.Is(err, control.ErrVersionConflict) {
		t.Fatalf("stale CompareAndSwap() error = %v", err)
	}
	loaded, ok, err := store.Load(context.Background(), "rate:apiserver:query")
	if err != nil || !ok || loaded.Version != 1 {
		t.Fatalf("Load() = %+v, %v, %v", loaded, ok, err)
	}
	if err := store.Delete(context.Background(), "rate:apiserver:query", 1); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := store.Load(context.Background(), "rate:apiserver:query"); err != nil || ok {
		t.Fatalf("Load() after delete ok=%v err=%v", ok, err)
	}
}

func TestStoreUnavailableDoesNotFallback(t *testing.T) {
	store := NewStore(nil, nil)
	if _, _, err := store.Load(context.Background(), "state"); !errors.Is(err, control.ErrUnavailable) {
		t.Fatalf("Load() error = %v", err)
	}
}

func TestStoreCommandClaimAndPerInstanceResults(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	store := NewStore(client, keyspace.NewBuilderWithNamespace("ops:runtime"))
	ctx := context.Background()
	identity, err := control.ResolveInstanceIdentity("collection-server", "collection-0")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Heartbeat(ctx, identity, time.Minute); err != nil {
		t.Fatal(err)
	}
	secondGeneration := identity
	secondGeneration.Generation = "generation-2"
	if err := store.Heartbeat(ctx, secondGeneration, time.Minute); err != nil {
		t.Fatal(err)
	}
	firstKey := keyspace.NewBuilderWithNamespace("ops:runtime").BuildResilienceInstanceKey(identity.Component, identity.InstanceID, identity.Generation)
	secondKey := keyspace.NewBuilderWithNamespace("ops:runtime").BuildResilienceInstanceKey(identity.Component, identity.InstanceID, secondGeneration.Generation)
	if count, err := client.Exists(ctx, firstKey, secondKey).Result(); err != nil || count != 2 {
		t.Fatalf("generation heartbeat keys count=%d err=%v", count, err)
	}
	command := control.Command{RequestID: "request-1", ActionID: "resilience.drain_queue",
		Target: control.Target{Component: "collection-server", InstanceID: "all"}, Actor: control.ActionActor{OrgID: 9}, ExpiresAt: time.Now().Add(time.Minute)}
	if err := store.PublishCommand(ctx, command, time.Minute); err != nil {
		t.Fatal(err)
	}
	commands, err := store.ListCommands(ctx, "collection-server", "collection-0")
	if err != nil || len(commands) != 1 {
		t.Fatalf("ListCommands() = %+v, %v", commands, err)
	}
	claimID := control.ScopedRequestID(command.Actor.OrgID, command.RequestID)
	claimed, err := store.Claim(ctx, claimID, identity.InstanceID, time.Minute)
	if err != nil || !claimed {
		t.Fatalf("first Claim() = %v, %v", claimed, err)
	}
	claimed, _ = store.Claim(ctx, claimID, identity.InstanceID, time.Minute)
	if claimed {
		t.Fatal("second Claim() = true, want idempotent rejection")
	}
	result := control.CommandResult{RequestID: command.RequestID, ActionID: command.ActionID,
		OrgID: command.Actor.OrgID, Component: identity.Component, InstanceID: identity.InstanceID, Status: control.CommandStatusOK}
	if err := store.PutCommandResult(ctx, result, time.Minute); err != nil {
		t.Fatal(err)
	}
	results, err := store.ListCommandResults(ctx, command.Actor.OrgID, command.RequestID)
	if err != nil || len(results) != 1 || results[0].InstanceID != identity.InstanceID {
		t.Fatalf("ListCommandResults() = %+v, %v", results, err)
	}
	instances, err := store.ListInstances(ctx, identity.Component)
	if err != nil || len(instances) != 2 || instances[0].Generation == "" || instances[1].Generation == "" {
		t.Fatalf("ListInstances() = %+v, %v", instances, err)
	}
}
