package transactionplan

import (
	"testing"

	"github.com/FangcunMount/reliable-messaging/message"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/FangcunMount/reliable-messaging/transport"
	"github.com/stretchr/testify/require"
)

func TestActiveExecutionBoundaryChecksEveryBranchAndExactIdentity(t *testing.T) {
	const owner = "qs.mysql"
	var active, history []transport.RouteBinding
	var branches []transaction.BranchInput
	for _, topic := range []string{"qs.evaluation.lifecycle", "qs.plan.task"} {
		old, err := transport.PhysicalTopicV2(topic, "TRANSACTION", owner, "m3-1")
		require.NoError(t, err)
		current, err := transport.PhysicalTopicV3(topic, "TRANSACTION", owner, "m3-1")
		require.NoError(t, err)
		active = append(active, transport.RouteBinding{Topic: topic, PhysicalTopic: current, MessageType: "TRANSACTION", Owner: owner, RouteRevision: "m3-1", NamingScheme: transport.ReadableNamingV1})
		history = append(history, transport.RouteBinding{Topic: topic, PhysicalTopic: old, MessageType: "TRANSACTION", Owner: owner, RouteRevision: "m3-1"})
		m, err := message.New(message.Input{ID: topic, Producer: "qs-server", Destination: topic, EventType: "test", SchemaVersion: "v1", Scope: "org:7", ContentType: "application/json", OccurredAt: "2026-10-27T00:00:00Z", Payload: []byte(`{}`)})
		require.NoError(t, err)
		branches = append(branches, transaction.BranchInput{ID: topic, Message: m, Wire: []byte{0, 255, 1}, PhysicalTopic: current, RouteRevision: "m3-1"})
	}
	routes, err := transport.NewRouteResolver(active, history)
	require.NoError(t, err)
	input := transaction.PlanInput{Owner: owner, Scope: "org:7", OperationID: "operation", IdempotencyKey: "request", Branches: branches}
	plan, err := transaction.NewPlan(input)
	require.NoError(t, err)
	require.NoError(t, RequireActiveRoutes(plan, routes))
	require.ErrorIs(t, RequireActiveRoutes(plan, nil), transaction.ErrBinding)
	require.ErrorIs(t, RequireActiveRoutes(transaction.FrozenPlan{}, routes), transaction.ErrInvalidPlan)
	input = plan.Input()
	input.Branches[1].PhysicalTopic = history[1].PhysicalTopic
	mixed, err := transaction.NewPlan(input)
	require.NoError(t, err)
	require.ErrorIs(t, RequireActiveRoutes(mixed, routes), ErrHistoricalExecution, "a later historical branch cannot acquire a new fence")
	input.Branches[1].PhysicalTopic = "unregistered"
	foreign, err := transaction.NewPlan(input)
	require.NoError(t, err)
	require.ErrorIs(t, RequireActiveRoutes(foreign, routes), transaction.ErrBinding)
	input = plan.Input()
	input.Owner = "ai.mysql"
	wrongOwner, err := transaction.NewPlan(input)
	require.NoError(t, err)
	require.ErrorIs(t, RequireActiveRoutes(wrongOwner, routes), transaction.ErrBinding)
	// Ordinary v2 deployments keep their prior active capability semantics.
	input = plan.Input()
	for i := range input.Branches {
		input.Branches[i].PhysicalTopic = history[i].PhysicalTopic
	}
	legacy, err := transaction.NewPlan(input)
	require.NoError(t, err)
	legacyRoutes, err := transport.NewRouteResolver(history, nil)
	require.NoError(t, err)
	require.NoError(t, RequireActiveRoutes(legacy, legacyRoutes))
}
