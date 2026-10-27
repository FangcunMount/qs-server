package eventtransaction

import (
	"testing"

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
