package aibridge

import (
	"testing"

	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"github.com/FangcunMount/reliable-messaging/transport"
	"github.com/stretchr/testify/require"
)

func archiveTestRoutes(t *testing.T) *transport.RouteResolver {
	t.Helper()
	var rows []transport.RouteBinding
	for _, topic := range []string{app.CommandsTopic, app.AcksTopic} {
		physical, err := transport.PhysicalTopicV2(topic, transport.TransactionMessageType, "qs.mysql", "m3-1")
		require.NoError(t, err)
		rows = append(rows, transport.RouteBinding{Topic: topic, PhysicalTopic: physical, MessageType: transport.TransactionMessageType, Owner: "qs.mysql", RouteRevision: "m3-1"})
	}
	routes, err := transport.NewRouteResolver(rows, nil)
	require.NoError(t, err)
	return routes
}
