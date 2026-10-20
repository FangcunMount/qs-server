package aibridge

import (
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"github.com/FangcunMount/reliable-messaging/transport"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestTransactionEventInputRequiresExactOwnerTypeRevisionAndSubscription(t *testing.T) {
	physical, err := transport.PhysicalTopicV2(app.EventsTopic, transport.TransactionMessageType, "ai.mysql", "m3-1")
	require.NoError(t, err)
	base := transport.RouteBinding{Topic: app.EventsTopic, PhysicalTopic: physical, MessageType: transport.TransactionMessageType, Owner: "ai.mysql", RouteRevision: "m3-1", Subscription: "qs-server.ai-events.v1"}
	for _, name := range []string{"valid", "owner", "type", "revision", "subscription", "physical", "multiple"} {
		t.Run(name, func(t *testing.T) {
			r := base
			switch name {
			case "owner":
				r.Owner = "qs.mysql"
			case "type":
				r.MessageType = transport.NormalMessageType
			case "revision":
				r.RouteRevision = "future"
			case "subscription":
				r.Subscription = "other"
			case "physical":
				r.PhysicalTopic = transport.PhysicalTopic(app.EventsTopic)
			}
			routes := []transport.RouteBinding{r}
			if name == "multiple" {
				routes = append(routes, base)
			}
			selected, actual, err := transactionalEventRoutes(routes)
			if name == "valid" {
				require.NoError(t, err)
				require.Equal(t, physical, actual)
				require.Equal(t, routes, selected)
			} else {
				require.Error(t, err)
			}
		})
	}
}
