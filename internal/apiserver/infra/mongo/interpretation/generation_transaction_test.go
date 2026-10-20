package interpretation

import (
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/generation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	standard "github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	catalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	"github.com/FangcunMount/reliable-messaging/message"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
)

func transactionLifecycleProof(t *testing.T, kind string) *evidence.EventEvidenceV2 {
	t.Helper()
	at := time.Date(2026, 10, 20, 1, 2, 3, 123456789, time.UTC)
	m, err := message.New(message.Input{ID: "generated-transaction", Producer: "qs-server", Destination: "report.lifecycle", EventType: kind, SchemaVersion: "v1", Scope: "org:1", ContentType: "application/json", OccurredAt: at.Format(time.RFC3339Nano), Payload: []byte(`{}`)})
	require.NoError(t, err)
	plan, err := transaction.NewPlan(transaction.PlanInput{Owner: "qs.mongo", Scope: "org:1", OperationID: "generation-operation", IdempotencyKey: "report/1", Branches: []transaction.BranchInput{{ID: "generated", Message: m, Wire: m.Input().Payload, PhysicalTopic: "fixture-transaction-topic", RouteRevision: "m3-1"}}})
	require.NoError(t, err)
	ref, err := plan.Reference("generated", 1, "m3-1")
	require.NoError(t, err)
	legacy := fixtureProof(t, kind, "fixture-business-binding")
	proof, err := evidence.NewTransactional(standard.ReferenceFromMessage(m), ref, legacy.BusinessBindingSHA256, at)
	require.NoError(t, err)
	return proof
}

func transactionGeneration(t *testing.T) *generation.ReportGeneration {
	t.Helper()
	proof := transactionLifecycleProof(t, catalog.InterpretationReportGenerated)
	at := proof.RecordedAt
	g, err := generation.New(1, generation.Key{OutcomeID: 9, ReportType: policy.ReportTypeStandard, TemplateVersion: "v1"}, at.Add(-time.Minute))
	require.NoError(t, err)
	require.NoError(t, g.Begin(2, at.Add(-time.Second)))
	require.NoError(t, g.SucceedWithTransactionEvidence(2, 3, at, proof))
	return g
}

func TestGenerationTransactionBSONRetainsCanonicalSDKBytesAndExplicitProtocol(t *testing.T) {
	mapper := NewLifecycleMapper()
	g := transactionGeneration(t)
	po := mapper.GenerationToPO(g)
	require.Nil(t, po.GeneratedEventEvidence)
	raw, err := bson.Marshal(po)
	require.NoError(t, err)
	var decoded ReportGenerationPO
	require.NoError(t, bson.Unmarshal(raw, &decoded))
	require.Nil(t, decoded.GeneratedEventEvidence)
	require.Equal(t, 2, decoded.GeneratedTransactionEvidence.Version)
	restored, err := mapper.GenerationToDomain(&decoded)
	require.NoError(t, err)
	require.Equal(t, g.GeneratedEventID(), restored.GeneratedEventID())
	require.Equal(t, g.GeneratedTransactionEvidence().TransactionReference, restored.GeneratedTransactionEvidence().TransactionReference)
	require.True(t, g.UpdatedAt().UTC().Truncate(time.Millisecond).Equal(restored.UpdatedAt()))
	decoded.GeneratedTransactionEvidence.TransactionReference[0] ^= 1
	require.NoError(t, restored.GeneratedTransactionEvidence().Validate())
}

func TestGenerationTransactionReadRejectsMixedMissingWrongOwnerAndUnknownSchema(t *testing.T) {
	for _, kind := range []string{"mixed", "missing-id", "wrong-owner", "unknown-schema", "wrong-time"} {
		t.Run(kind, func(t *testing.T) {
			mapper := NewLifecycleMapper()
			po := mapper.GenerationToPO(transactionGeneration(t))
			switch kind {
			case "mixed":
				po.GeneratedEventEvidence = fixtureProof(t, catalog.InterpretationReportGenerated, po.GeneratedEventID)
			case "missing-id":
				po.GeneratedEventID = ""
			case "wrong-owner":
				ref, err := transaction.DecodeReference(po.GeneratedTransactionEvidence.TransactionReference)
				require.NoError(t, err)
				ref.Owner = "qs.mysql"
				po.GeneratedTransactionEvidence.TransactionReference, err = transaction.EncodeReference(ref)
				require.NoError(t, err)
			case "unknown-schema":
				po.TransactionSchemaVersion = 0
			case "wrong-time":
				po.UpdatedAt = po.UpdatedAt.Add(time.Millisecond)
			}
			_, err := mapper.GenerationToDomain(po)
			require.Error(t, err)
		})
	}
}

func TestGenerationTransactionWriteRejectsAbsentOriginalSessionBeforeIO(t *testing.T) {
	r := &GenerationRepository{mapper: NewLifecycleMapper()}
	g := transactionGeneration(t)
	require.ErrorIs(t, r.Create(t.Context(), g), transaction.ErrTransactionRequired)
	require.ErrorIs(t, r.Save(t.Context(), g, g.Version()-1), transaction.ErrTransactionRequired)
}
