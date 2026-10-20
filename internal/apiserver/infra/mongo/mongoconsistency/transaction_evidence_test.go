package mongoconsistency

import (
	"context"
	"strings"
	"testing"
	"time"

	appaudit "github.com/FangcunMount/qs-server/internal/apiserver/application/mongoconsistency"
	standard "github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	shared "github.com/FangcunMount/qs-server/internal/apiserver/eventing/transactionplan"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/eventtransaction"
	"github.com/FangcunMount/qs-server/internal/pkg/event"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/mongo"
)

type auditOwnerStub struct {
	plan   transaction.FrozenPlan
	fact   *transaction.Evidence
	err    error
	sameDB bool
}

func (o auditOwnerStub) UsesDatabase(*mongo.Database) bool { return o.sameDB }
func (o auditOwnerStub) PlanForReference(context.Context, transaction.Reference) (transaction.FrozenPlan, error) {
	return o.plan, o.err
}
func (o auditOwnerStub) Evidence(context.Context, transaction.Reference) (*transaction.Evidence, error) {
	return o.fact, o.err
}

func transactionAuditFixture(t *testing.T) (*Scanner, *evidence.EventEvidenceV2, auditOwnerStub) {
	t.Helper()
	evt := event.New("interpretation.report.generated", "ReportGeneration", "1", struct {
		OrgID int64 `json:"org_id"`
	}{7})
	plan, err := shared.Freeze(port.Request{Key: "first", Artifact: []byte(`{}`), Events: []event.DomainEvent{evt}}, storedRowTestResolver{}, "qs.mongo", "m3-1")
	require.NoError(t, err)
	b := plan.Input().Branches[0]
	ref, err := plan.Reference(b.ID, 1, "m3-1")
	require.NoError(t, err)
	proof, err := evidence.NewTransactional(standard.ReferenceFromMessage(b.Message), ref, strings.Repeat("a", 64), time.Now())
	require.NoError(t, err)
	owner := auditOwnerStub{plan: plan, fact: &transaction.Evidence{Reference: ref, ExecutionState: transaction.Committed, BranchState: transaction.Committed}, sameDB: true}
	return &Scanner{transactionOwner: owner}, proof, owner
}

func TestV2AuditRequiresExactAuthorityAndOriginalTypedWire(t *testing.T) {
	for _, kind := range []string{"healthy", "prepared", "missing", "wrong-fence", "different-plan", "wrong-aggregate", "wrong-binding", "db-timeout", "binding-config", "wrong-database"} {
		t.Run(kind, func(t *testing.T) {
			s, proof, owner := transactionAuditFixture(t)
			aggregateID := "1"
			actualBinding := proof.BusinessBindingSHA256
			switch kind {
			case "prepared":
				owner.fact.BranchState = transaction.Prepared
			case "missing":
				owner.fact = nil
			case "wrong-fence":
				owner.fact.Reference.Fence++
			case "different-plan":
				in := owner.plan.Input()
				in.Branches[0].Wire = []byte("replacement wire")
				var err error
				owner.plan, err = transaction.NewPlan(in)
				require.NoError(t, err)
			case "wrong-aggregate":
				aggregateID = "2"
			case "wrong-binding":
				actualBinding = strings.Repeat("b", 64)
			case "db-timeout":
				owner.err = context.DeadlineExceeded
			case "binding-config":
				owner.err = transaction.ErrBinding
			case "wrong-database":
				owner.sameDB = false
			}
			s.transactionOwner = owner
			err := s.checkTransactionEvidence(t.Context(), proof, proof.Reference.EventID, proof.Reference.EventType, "ReportGeneration", aggregateID, "org:7", proof.BusinessBindingSHA256, func([]byte) (string, error) { return actualBinding, nil })
			if kind == "healthy" {
				require.NoError(t, err)
				var result appaudit.BatchResult
				countTransactionEvidence(&result)
				require.Equal(t, int64(1), result.EvidenceClasses["transaction_reference"])
				return
			}
			require.Error(t, err)
			if kind == "db-timeout" || kind == "binding-config" || kind == "wrong-database" {
				require.False(t, isEvidenceDrift(err), "configuration/DB failure must preserve checkpoint")
			} else {
				require.True(t, isEvidenceDrift(err))
			}
		})
	}
}

func TestV2AuditWithoutConfiguredOwnerNeverFallsBackToOutbox(t *testing.T) {
	_, proof, _ := transactionAuditFixture(t)
	err := (&Scanner{}).checkTransactionEvidence(t.Context(), proof, proof.Reference.EventID, proof.Reference.EventType, "ReportGeneration", "1", "org:7", proof.BusinessBindingSHA256, nil)
	require.Error(t, err)
	require.False(t, isEvidenceDrift(err))
}
