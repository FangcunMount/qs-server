package eventtransaction

import (
	"bytes"
	"crypto/sha256"
	"testing"

	standard "github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/eventtransaction"
	"github.com/FangcunMount/qs-server/internal/pkg/event"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/stretchr/testify/require"
)

type fixtureResolver struct{}

func (fixtureResolver) GetTopicForEvent(string) (string, bool) {
	return "qs.evaluation.lifecycle", true
}

func fixtureRequest(key string) port.Request {
	e := event.New("evaluation.failed", "Assessment", "42", struct {
		OrgID int64  `json:"org_id"`
		Value string `json:"value"`
	}{7, "original"})
	return port.Request{Key: key, InputHash: sha256.Sum256([]byte("stable-input")), Artifact: []byte("{ \"raw\": \"<>&\", \"value\":1.00 }\n"), Events: []event.DomainEvent{e}}
}

func TestPlanPreservesArtifactAndOriginalConsumerWire(t *testing.T) {
	q := fixtureRequest("frozen")
	wire, err := standard.EncodeWire(q.Events[0], "api-server")
	require.NoError(t, err)
	p, err := freeze(q, fixtureResolver{})
	require.NoError(t, err)
	raw, err := Artifact(p)
	require.NoError(t, err)
	require.Equal(t, q.Artifact, raw)
	require.Equal(t, wire, p.Input().Branches[0].Wire)
	require.False(t, bytes.Equal(p.Input().Branches[0].Message.Input().Payload, wire))
	encoded, err := transaction.EncodePlan(p)
	require.NoError(t, err)
	reloaded, err := transaction.DecodePlan(encoded)
	require.NoError(t, err)
	raw, err = Artifact(reloaded)
	require.NoError(t, err)
	require.Equal(t, q.Artifact, raw)
	require.Equal(t, wire, reloaded.Input().Branches[0].Wire)
}

func TestPlanRejectsCrossTenantAndExcessBranchesBeforeReservation(t *testing.T) {
	q := fixtureRequest("mixed")
	q.Events = append(q.Events, event.New("evaluation.failed", "Assessment", "43", struct {
		OrgID int64 `json:"org_id"`
	}{8}))
	_, err := freeze(q, fixtureResolver{})
	require.Error(t, err)
	q = fixtureRequest("excess")
	for len(q.Events) <= transaction.MaxBranches {
		q.Events = append(q.Events, q.Events[0])
	}
	_, err = freeze(q, fixtureResolver{})
	require.Error(t, err)
}

func TestPlanRejectsArtifactOrRouteSubstitution(t *testing.T) {
	p, err := freeze(fixtureRequest("substitute"), fixtureResolver{})
	require.NoError(t, err)
	for _, mutate := range []func(*transaction.PlanInput){
		func(in *transaction.PlanInput) { in.Owner = "qs.mongo" },
		func(in *transaction.PlanInput) { in.Branches[0].Wire = []byte("other") },
		func(in *transaction.PlanInput) { in.Branches[0].PhysicalTopic = "other" },
		func(in *transaction.PlanInput) { in.Branches[0].RouteRevision = "other" },
	} {
		in := p.Input()
		mutate(&in)
		changed, err := transaction.NewPlan(in)
		require.NoError(t, err)
		_, err = Artifact(changed)
		require.Error(t, err)
	}
}
