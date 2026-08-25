package redisadapter

import (
	"context"
	"encoding/json"
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
	identity, err := control.ResolveInstanceIdentity("apiserver", "api-0")
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
	command := control.Command{RequestID: "request-1", ActionID: "resilience.release_lock",
		Target: control.Target{Component: "apiserver", InstanceID: "all"}, Actor: control.ActionActor{OrgID: 9}, ExpiresAt: time.Now().Add(time.Minute)}
	if err := store.PublishCommand(ctx, command, time.Minute); err != nil {
		t.Fatal(err)
	}
	commandIndex := keyspace.NewBuilderWithNamespace("ops:runtime").BuildResilienceCommandIndexKey(command.Target.Component)
	if count, err := client.ZCard(ctx, commandIndex).Result(); err != nil || count != 1 {
		t.Fatalf("command index count=%d err=%v", count, err)
	}
	duplicate := command
	duplicate.ActionID = "must-not-replace-first-command"
	if err := store.PublishCommand(ctx, duplicate, 2*time.Minute); err != nil {
		t.Fatal(err)
	}
	commandKey := keyspace.NewBuilderWithNamespace("ops:runtime").BuildResilienceCommandKey(command.Target.Component, control.ScopedRequestID(command.Actor.OrgID, command.RequestID))
	rawCommand, err := client.Get(ctx, commandKey).Bytes()
	if err != nil {
		t.Fatal(err)
	}
	var storedCommand control.Command
	if err := json.Unmarshal(rawCommand, &storedCommand); err != nil || storedCommand.ActionID != command.ActionID {
		t.Fatalf("duplicate publish replaced first command: command=%+v err=%v", storedCommand, err)
	}
	commands, err := store.ListCommands(ctx, "apiserver", "api-0")
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

func TestStoreIndexedReadsPruneExpiredAndMissingValues(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	builder := keyspace.NewBuilderWithNamespace("ops:runtime")
	store := NewStore(client, builder)
	ctx := context.Background()

	identity, err := control.ResolveInstanceIdentity("apiserver", "api-expiring")
	if err != nil {
		t.Fatal(err)
	}
	command := control.Command{
		RequestID: "request-expiring", ActionID: "resilience.release_lock",
		Target: control.Target{Component: "apiserver", InstanceID: identity.InstanceID},
		Actor:  control.ActionActor{OrgID: 9}, ExpiresAt: time.Now().Add(time.Second),
	}
	result := control.CommandResult{
		RequestID: command.RequestID, OrgID: command.Actor.OrgID, ActionID: command.ActionID,
		Component: identity.Component, InstanceID: identity.InstanceID, Status: control.CommandStatusOK,
	}
	if err := store.Heartbeat(ctx, identity, time.Second); err != nil {
		t.Fatal(err)
	}
	if err := store.PublishCommand(ctx, command, time.Second); err != nil {
		t.Fatal(err)
	}
	if err := store.PutCommandResult(ctx, result, time.Second); err != nil {
		t.Fatal(err)
	}

	mr.FastForward(2 * time.Second)
	commands, commandErr := store.ListCommands(ctx, identity.Component, identity.InstanceID)
	results, resultErr := store.ListCommandResults(ctx, command.Actor.OrgID, command.RequestID)
	instances, instanceErr := store.ListInstances(ctx, identity.Component)
	if commandErr != nil || resultErr != nil || instanceErr != nil || len(commands) != 0 || len(results) != 0 || len(instances) != 0 {
		t.Fatalf("expired indexed values commands=%v results=%v instances=%v errors=%v/%v/%v", commands, results, instances, commandErr, resultErr, instanceErr)
	}

	missingKey := builder.BuildResilienceCommandKey(identity.Component, "9:missing")
	commandIndex := builder.BuildResilienceCommandIndexKey(identity.Component)
	if err := client.ZAdd(ctx, commandIndex, redis.Z{Score: float64(time.Now().Add(time.Minute).UnixMilli()), Member: missingKey}).Err(); err != nil {
		t.Fatal(err)
	}
	if commands, err := store.ListCommands(ctx, identity.Component, identity.InstanceID); err != nil || len(commands) != 0 {
		t.Fatalf("ListCommands() with orphaned index = %v, %v", commands, err)
	}
	if count, err := client.ZCard(ctx, commandIndex).Result(); err != nil || count != 0 {
		t.Fatalf("orphaned command index count=%d err=%v", count, err)
	}
}
