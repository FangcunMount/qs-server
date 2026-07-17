package redisadapter

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/resiliencecontrol"
	"github.com/alicebob/miniredis/v2"
	redis "github.com/redis/go-redis/v9"
)

func TestStoreCompareAndSwapAndDelete(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	store := NewStore(client, keyspace.NewBuilderWithNamespace("ops:runtime"))

	published, err := store.CompareAndSwap(context.Background(), "rate:apiserver:query", 0, resiliencecontrol.VersionedState{Payload: []byte(`{"qps":10}`)}, time.Minute)
	if err != nil || published.Version != 1 {
		t.Fatalf("CompareAndSwap() = %+v, %v", published, err)
	}
	if _, err := store.CompareAndSwap(context.Background(), "rate:apiserver:query", 0, resiliencecontrol.VersionedState{}, time.Minute); !errors.Is(err, resiliencecontrol.ErrVersionConflict) {
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
	if _, _, err := store.Load(context.Background(), "state"); !errors.Is(err, resiliencecontrol.ErrUnavailable) {
		t.Fatalf("Load() error = %v", err)
	}
}

func TestStoreCommandClaimAndPerInstanceResults(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	store := NewStore(client, keyspace.NewBuilderWithNamespace("ops:runtime"))
	ctx := context.Background()
	identity := resiliencecontrol.ResolveInstanceIdentity("collection-server", "collection-0")
	if err := store.Heartbeat(ctx, identity, time.Minute); err != nil {
		t.Fatal(err)
	}
	command := resiliencecontrol.Command{RequestID: "request-1", ActionID: "resilience.drain_queue",
		Target: resiliencecontrol.Target{Component: "collection-server", InstanceID: "all"}, Actor: resiliencecontrol.ActionActor{OrgID: 9}, ExpiresAt: time.Now().Add(time.Minute)}
	if err := store.PublishCommand(ctx, command, time.Minute); err != nil {
		t.Fatal(err)
	}
	commands, err := store.ListCommands(ctx, "collection-server", "collection-0")
	if err != nil || len(commands) != 1 {
		t.Fatalf("ListCommands() = %+v, %v", commands, err)
	}
	claimID := resiliencecontrol.ScopedRequestID(command.Actor.OrgID, command.RequestID)
	claimed, err := store.Claim(ctx, claimID, identity.InstanceID, time.Minute)
	if err != nil || !claimed {
		t.Fatalf("first Claim() = %v, %v", claimed, err)
	}
	claimed, _ = store.Claim(ctx, claimID, identity.InstanceID, time.Minute)
	if claimed {
		t.Fatal("second Claim() = true, want idempotent rejection")
	}
	result := resiliencecontrol.CommandResult{RequestID: command.RequestID, ActionID: command.ActionID,
		OrgID: command.Actor.OrgID, Component: identity.Component, InstanceID: identity.InstanceID, Status: resiliencecontrol.CommandStatusOK}
	if err := store.PutCommandResult(ctx, result, time.Minute); err != nil {
		t.Fatal(err)
	}
	results, err := store.ListCommandResults(ctx, command.Actor.OrgID, command.RequestID)
	if err != nil || len(results) != 1 || results[0].InstanceID != identity.InstanceID {
		t.Fatalf("ListCommandResults() = %+v, %v", results, err)
	}
	instances, err := store.ListInstances(ctx, identity.Component)
	if err != nil || len(instances) != 1 || instances[0].Generation == "" {
		t.Fatalf("ListInstances() = %+v, %v", instances, err)
	}
}
