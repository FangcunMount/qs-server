package execution

import (
	"encoding/hex"
	"strings"
	"testing"
	"time"

	catalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	"github.com/FangcunMount/reliable-messaging/message"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/stretchr/testify/require"
)

func TestCloneGenerationPreservesTransactionEvidenceWithoutPromotingToOutbox(t *testing.T) {
	_, g, run, _, _, _, _, at := commitFixture(t, &eventStagerStub{})
	in := message.Input{ID: "clone-generated-event", Producer: "qs-server", Destination: "report.lifecycle", EventType: catalog.InterpretationReportGenerated, SchemaVersion: "v1", Scope: "org:1", ContentType: "application/json", OccurredAt: at.Format(time.RFC3339Nano), Payload: []byte(`{}`)}
	m, err := message.New(in)
	require.NoError(t, err)
	p, err := transaction.NewPlan(transaction.PlanInput{Owner: "qs.mongo", Scope: in.Scope, OperationID: "report-clone", IdempotencyKey: "report/clone", Branches: []transaction.BranchInput{{ID: "generated", Message: m, Wire: in.Payload, PhysicalTopic: "fixture-transaction-topic", RouteRevision: "m3-1"}}})
	require.NoError(t, err)
	ref, err := p.Reference("generated", 1, "m3-1")
	require.NoError(t, err)
	fp := m.Fingerprint()
	standard := evidence.StandardReference{EventID: in.ID, Producer: in.Producer, Destination: in.Destination, EventType: in.EventType, SchemaVersion: in.SchemaVersion, Scope: in.Scope, ContentType: in.ContentType, OccurredAt: in.OccurredAt, Fingerprint: hex.EncodeToString(fp[:])}
	proof, err := evidence.NewTransactional(standard, ref, strings.Repeat("b", 64), at)
	require.NoError(t, err)
	require.NoError(t, g.SucceedWithTransactionEvidence(run.ID(), 900, at, proof))
	copy, err := cloneGeneration(g)
	require.NoError(t, err)
	require.Equal(t, g.GeneratedEventID(), copy.GeneratedEventID())
	require.Equal(t, g.GeneratedTransactionEvidence(), copy.GeneratedTransactionEvidence())
	require.Nil(t, copy.GeneratedEventEvidence())
	getter := copy.GeneratedTransactionEvidence()
	getter.TransactionReference[0] ^= 1
	require.NoError(t, g.GeneratedTransactionEvidence().Validate())
	require.NoError(t, copy.GeneratedTransactionEvidence().Validate())
}
