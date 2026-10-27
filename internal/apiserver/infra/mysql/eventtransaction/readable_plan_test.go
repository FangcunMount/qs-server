package eventtransaction

import (
	"testing"
	"time"

	shared "github.com/FangcunMount/qs-server/internal/apiserver/eventing/transactionplan"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/FangcunMount/reliable-messaging/transport"
	"github.com/stretchr/testify/require"
)

func TestManifestSelectsNewPlansButOnlyReadsExactHistoricalArtifacts(t *testing.T) {
	const topic = "qs.evaluation.lifecycle"
	physical, err := transport.PhysicalTopicV3(topic, transport.TransactionMessageType, owner, revision)
	require.NoError(t, err)
	active := transport.RouteBinding{Topic: topic, PhysicalTopic: physical, MessageType: transport.TransactionMessageType, Owner: owner, RouteRevision: revision, NamingScheme: transport.ReadableNamingV1}
	old := active
	old.NamingScheme = ""
	old.PhysicalTopic, err = transport.PhysicalTopicV2(topic, transport.TransactionMessageType, owner, revision)
	require.NoError(t, err)
	routes, err := transport.NewRouteResolver([]transport.RouteBinding{active}, []transport.RouteBinding{old})
	require.NoError(t, err)
	q := fixtureRequest("same-business")
	before, err := freeze(q, fixtureResolver{})
	require.NoError(t, err)
	after, err := shared.FreezeWithRoutes(q, fixtureResolver{}, owner, revision, routes)
	require.NoError(t, err)
	require.Equal(t, physical, after.Input().Branches[0].PhysicalTopic)
	require.Equal(t, before.Input().OperationID, after.Input().OperationID)
	require.Equal(t, before.Input().Branches[0].Wire, after.Input().Branches[0].Wire)
	require.Equal(t, before.Input().Branches[0].Message.Fingerprint(), after.Input().Branches[0].Message.Fingerprint())
	require.NotEqual(t, before.Digest(), after.Digest())
	for _, p := range []transaction.FrozenPlan{before, after} {
		body, err := shared.ArtifactWithRoutes(p, owner, revision, routes)
		require.NoError(t, err)
		require.Equal(t, q.Artifact, body)
	}
	activeOnly, err := transport.NewRouteResolver([]transport.RouteBinding{active}, nil)
	require.NoError(t, err)
	_, err = shared.ArtifactWithRoutes(before, owner, revision, activeOnly)
	require.Error(t, err, "unregistered historical topic must not be accepted")
	_, err = shared.FreezeWithRoutes(q, fixtureResolver{}, owner, revision, nil)
	require.ErrorIs(t, err, transaction.ErrBinding)
	changed := after.Input()
	changed.Branches[0].PhysicalTopic = "other"
	wrong, err := transaction.NewPlan(changed)
	require.NoError(t, err)
	_, err = shared.ArtifactWithRoutes(wrong, owner, revision, routes)
	require.Error(t, err)
}

func TestHistoricalRetryCannotAllocateFenceOrTouchPrimary(t *testing.T) {
	const topic = "qs.evaluation.lifecycle"
	old, err := transport.PhysicalTopicV2(topic, transport.TransactionMessageType, owner, revision)
	require.NoError(t, err)
	current, err := transport.PhysicalTopicV3(topic, transport.TransactionMessageType, owner, revision)
	require.NoError(t, err)
	routes, err := transport.NewRouteResolver([]transport.RouteBinding{{Topic: topic, PhysicalTopic: current, MessageType: transport.TransactionMessageType, Owner: owner, RouteRevision: revision, NamingScheme: transport.ReadableNamingV1}}, []transport.RouteBinding{{Topic: topic, PhysicalTopic: old, MessageType: transport.TransactionMessageType, Owner: owner, RouteRevision: revision}})
	require.NoError(t, err)
	q := fixtureRequest("historical-retry")
	plan, err := freeze(q, fixtureResolver{})
	require.NoError(t, err)
	committer := &Committer{routes: routes, resolver: fixtureResolver{}, topics: map[string]bool{topic: true}}
	// No primary, store or client exists. Historic reads still work, but even an
	// explicit ordinary Retry must fail before any DB lookup or fence mutation.
	require.NoError(t, committer.validatePlan(plan))
	before, err := transaction.EncodePlan(plan)
	require.NoError(t, err)
	_, err = committer.RetryAborted(t.Context(), transaction.Execution{Plan: plan}, transaction.ReserveOptions{Lease: time.Minute})
	require.ErrorIs(t, err, shared.ErrHistoricalExecution)
	after, err := transaction.EncodePlan(plan)
	require.NoError(t, err)
	require.Equal(t, before, after)
}

func TestExplicitRouteReplanRetainsFirstArtifactAndWire(t *testing.T) {
	const topic = "qs.evaluation.lifecycle"
	old, err := transport.PhysicalTopicV2(topic, transport.TransactionMessageType, owner, revision)
	require.NoError(t, err)
	current, err := transport.PhysicalTopicV3(topic, transport.TransactionMessageType, owner, revision)
	require.NoError(t, err)
	routes, err := transport.NewRouteResolver([]transport.RouteBinding{{Topic: topic, PhysicalTopic: current, MessageType: transport.TransactionMessageType, Owner: owner, RouteRevision: revision, NamingScheme: transport.ReadableNamingV1}}, []transport.RouteBinding{{Topic: topic, PhysicalTopic: old, MessageType: transport.TransactionMessageType, Owner: owner, RouteRevision: revision}})
	require.NoError(t, err)
	q := fixtureRequest("explicit-route-replan")
	previous, err := freeze(q, fixtureResolver{})
	require.NoError(t, err)
	original, err := transaction.EncodePlan(previous)
	require.NoError(t, err)
	input := previous.Input()
	input.OperationID = "explicit-new-operation"
	input.Branches[0].PhysicalTopic = current
	candidate, err := transaction.NewPlan(input)
	require.NoError(t, err)
	authorization := transaction.ReplanAuthorization{Previous: transaction.Execution{Plan: previous, Fence: 1, WriterEpoch: "m3-1"}, Candidate: candidate}
	// This pure host-plan proof never calls Reserve/Retry. Actual Aborted CAS,
	// retained prior execution and duplicate/Unknown refusal are SDK store gates.
	require.NotEqual(t, authorization.Previous.Plan.Input().OperationID, authorization.Candidate.Input().OperationID)
	require.Equal(t, previous.Input().InputHash, candidate.Input().InputHash)
	require.Equal(t, previous.Input().IdempotencyKey, candidate.Input().IdempotencyKey)
	require.Equal(t, previous.Input().Branches[0].Wire, candidate.Input().Branches[0].Wire)
	require.Equal(t, previous.Input().Branches[0].Message.Fingerprint(), candidate.Input().Branches[0].Message.Fingerprint())
	for _, plan := range []transaction.FrozenPlan{previous, candidate} {
		body, err := shared.ArtifactWithRoutes(plan, owner, revision, routes)
		require.NoError(t, err)
		require.Equal(t, q.Artifact, body)
	}
	require.ErrorIs(t, shared.RequireActiveRoutes(previous, routes), shared.ErrHistoricalExecution)
	require.NoError(t, shared.RequireActiveRoutes(candidate, routes))
	after, err := transaction.EncodePlan(previous)
	require.NoError(t, err)
	require.Equal(t, original, after)
}
