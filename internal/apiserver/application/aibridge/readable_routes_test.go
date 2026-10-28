package aibridge

import (
	"testing"

	"github.com/FangcunMount/reliable-messaging/transport"
	"github.com/stretchr/testify/require"
)

func messagingTestRoutes(t *testing.T) *transport.RouteResolver {
	return messagingRoutesForTest(t, false)
}
func messagingRoutesForTest(t *testing.T, readable bool) *transport.RouteResolver {
	t.Helper()
	var active, history []transport.RouteBinding
	for _, topic := range []string{CommandsTopic, AcksTopic} {
		physical, err := transport.PhysicalTopicV2(topic, transport.TransactionMessageType, commandOwner, commandRevision)
		require.NoError(t, err)
		old := transport.RouteBinding{Topic: topic, PhysicalTopic: physical, MessageType: transport.TransactionMessageType, Owner: commandOwner, RouteRevision: commandRevision}
		row := old
		if readable {
			row.NamingScheme = transport.ReadableNamingV1
			row.PhysicalTopic, err = transport.PhysicalTopicV3(topic, transport.TransactionMessageType, commandOwner, commandRevision)
			require.NoError(t, err)
			history = append(history, old)
		}
		active = append(active, row)
	}
	routes, err := transport.NewRouteResolver(active, history)
	require.NoError(t, err)
	return routes
}
func TestReadableCommandUsesManifestWithoutSealingAgainAndReadsExactOldPlan(t *testing.T) {
	c, message := commandFixture(t)
	old, err := c.Freeze(message, 1, messagingTestRoutes(t))
	require.NoError(t, err)
	routes := messagingRoutesForTest(t, true)
	current, err := c.Freeze(message, 1, routes)
	require.NoError(t, err)
	require.Equal(t, "qs-ai-commands-v1--tx--qs-mysql--m3-1", current.Input().Branches[0].PhysicalTopic)
	require.Equal(t, old.Input().Branches[0].Wire, current.Input().Branches[0].Wire)
	require.Equal(t, old.Input().OperationID, current.Input().OperationID)
	require.NotEqual(t, old.Digest(), current.Digest())
	_, archived, err := ReadTransactionCommand(old, routes)
	require.NoError(t, err)
	require.Equal(t, message.Wire, archived.Wire)
	_, archived, err = ReadTransactionCommand(current, routes)
	require.NoError(t, err)
	require.Equal(t, message.Wire, archived.Wire)
	_, err = c.Freeze(message, 1, nil)
	require.Error(t, err)
	_, _, err = ReadTransactionCommand(current, messagingTestRoutes(t))
	require.Error(t, err, "unregistered physical resource accepted")
}
