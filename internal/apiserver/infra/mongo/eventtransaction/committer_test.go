package eventtransaction

import (
	"context"
	"crypto/sha256"
	"testing"

	shared "github.com/FangcunMount/qs-server/internal/apiserver/eventing/transactionplan"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/eventtransaction"
	"github.com/FangcunMount/qs-server/internal/pkg/event"
	catalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"github.com/FangcunMount/qs-server/internal/pkg/messagingruntime"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/stretchr/testify/require"
)

type reportRoutes struct{}

func (reportRoutes) GetTopicForEvent(kind string) (string, bool) {
	return "reports.lifecycle", kind == catalog.InterpretationReportGenerated
}

func TestSharedCodecKeepsOriginalPrivateArtifactAndWireAcrossOwners(t *testing.T) {
	owner, _, db, _, _ := ownerFixture(t)
	runner := runnerFunc(func(context.Context, func(context.Context) error) error {
		t.Fatal("unexpected original root")
		return nil
	})
	c, err := NewCommitter(db, owner, runner, reportRoutes{})
	require.NoError(t, err)
	q := port.Request{Key: "report/1", InputHash: sha256.Sum256([]byte("frozen input")), Artifact: []byte("{ \"report\":\"<>&\",\"value\":1.00 }\n"), Events: []event.DomainEvent{event.New(catalog.InterpretationReportGenerated, "ReportGeneration", "1", struct {
		OrgID int64 `json:"org_id"`
	}{1})}}
	mongoPlan, err := shared.Freeze(q, reportRoutes{}, ownerName, routeRevision)
	require.NoError(t, err)
	mysqlPlan, err := shared.Freeze(q, reportRoutes{}, "qs.mysql", routeRevision)
	require.NoError(t, err)
	body, err := c.Artifact(mongoPlan)
	require.NoError(t, err)
	require.Equal(t, q.Artifact, body)
	require.Equal(t, mysqlPlan.Input().Branches[0].Wire, mongoPlan.Input().Branches[0].Wire)
	require.Equal(t, mysqlPlan.Input().Branches[0].Message.Input().Payload, mongoPlan.Input().Branches[0].Message.Input().Payload)
	require.NotEqual(t, mysqlPlan.Digest(), mongoPlan.Digest())
	_, err = c.Artifact(mysqlPlan)
	require.Error(t, err)
	raw, err := transaction.EncodePlan(mongoPlan)
	require.NoError(t, err)
	decoded, err := transaction.DecodePlan(raw)
	require.NoError(t, err)
	require.NoError(t, c.validatePlan(decoded))
	_, err = c.Reserve(t.Context(), q)
	require.ErrorIs(t, err, messagingruntime.ErrTransactionAdmissionClosed)
	_, err = NewCommitter(db.Client().Database("wrong_database"), owner, runner, reportRoutes{})
	require.ErrorIs(t, err, transaction.ErrBinding)
	_, err = NewCommitter(db, owner, nil, reportRoutes{})
	require.ErrorIs(t, err, transaction.ErrBinding)
}
