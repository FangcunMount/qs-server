//go:build integration

package redisadapter

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/control"
	redis "github.com/redis/go-redis/v9"
)

func TestStoreIndexedOperationsAgainstRedis7(t *testing.T) {
	redisURL := os.Getenv("QS_SERVER_TEST_REDIS_URL")
	if redisURL == "" {
		t.Skip("QS_SERVER_TEST_REDIS_URL is required")
	}
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		t.Fatal(err)
	}
	client := redis.NewClient(opts)
	t.Cleanup(func() { _ = client.Close() })
	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Fatal(err)
	}

	namespace := fmt.Sprintf("integration:resilience:%d", time.Now().UnixNano())
	builder := keyspace.NewBuilderWithNamespace(namespace)
	store := NewStore(client, builder)
	identity, err := control.ResolveInstanceIdentity("apiserver", "api-integration")
	if err != nil {
		t.Fatal(err)
	}
	command := control.Command{
		RequestID: "request-1", ActionID: "resilience.release_lock",
		Target: control.Target{Component: identity.Component, InstanceID: identity.InstanceID},
		Actor:  control.ActionActor{OrgID: 9}, ExpiresAt: time.Now().Add(time.Minute),
	}
	result := control.CommandResult{
		RequestID: command.RequestID, OrgID: command.Actor.OrgID, ActionID: command.ActionID,
		Component: identity.Component, InstanceID: identity.InstanceID, Status: control.CommandStatusOK,
	}
	scopedRequestID := control.ScopedRequestID(command.Actor.OrgID, command.RequestID)
	keys := []string{
		builder.BuildResilienceCommandKey(identity.Component, scopedRequestID),
		builder.BuildResilienceCommandIndexKey(identity.Component),
		builder.BuildResilienceCommandResultKey(scopedRequestID, identity.InstanceID),
		builder.BuildResilienceCommandResultIndexKey(scopedRequestID),
		builder.BuildResilienceInstanceKey(identity.Component, identity.InstanceID, identity.Generation),
		builder.BuildResilienceInstanceIndexKey(identity.Component),
	}
	t.Cleanup(func() { _ = client.Del(context.Background(), keys...).Err() })

	if err := store.Heartbeat(ctx, identity, time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := store.PublishCommand(ctx, command, time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := store.PutCommandResult(ctx, result, time.Minute); err != nil {
		t.Fatal(err)
	}
	commands, commandErr := store.ListCommands(ctx, identity.Component, identity.InstanceID)
	results, resultErr := store.ListCommandResults(ctx, command.Actor.OrgID, command.RequestID)
	instances, instanceErr := store.ListInstances(ctx, identity.Component)
	if commandErr != nil || resultErr != nil || instanceErr != nil || len(commands) != 1 || len(results) != 1 || len(instances) != 1 {
		t.Fatalf("indexed Redis operations commands=%v results=%v instances=%v errors=%v/%v/%v", commands, results, instances, commandErr, resultErr, instanceErr)
	}
}
