package messagingruntime

import (
	"testing"

	"github.com/FangcunMount/reliable-messaging/transport"
	"github.com/stretchr/testify/require"
)

func TestNormalPublisherSeparatesMixedPhysicalInputsAndRetainsFanout(t *testing.T) {
	normal := transport.RouteBinding{Topic: "qs.plan.task", MessageType: transport.NormalMessageType, Owner: "qs.notify", RouteRevision: "m3-1", Subscription: "worker"}
	other := normal
	other.Subscription = "projection"
	tx := normal
	tx.MessageType = transport.TransactionMessageType
	tx.Owner = "qs.mysql"
	selected, err := normalRoutes([]transport.RouteBinding{tx, normal, other}, []string{"qs.plan.task"})
	require.NoError(t, err)
	require.Equal(t, []transport.RouteBinding{normal, other}, selected)
	_, err = normalRoutes([]transport.RouteBinding{tx}, []string{"qs.plan.task"})
	require.Error(t, err, "TRANSACTION cannot satisfy a NORMAL route")
	other.RouteRevision = "future"
	_, err = normalRoutes([]transport.RouteBinding{other}, []string{"qs.plan.task"})
	require.Error(t, err, "a new revision needs explicit host selection")
}
