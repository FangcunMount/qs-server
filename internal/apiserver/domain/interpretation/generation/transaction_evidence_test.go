package generation

import (
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	catalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	"github.com/FangcunMount/reliable-messaging/message"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/stretchr/testify/require"
)

func transactionProof(t *testing.T, at time.Time) *evidence.EventEvidenceV2 {
	t.Helper()
	in := message.Input{ID: "generated-event", Producer: "qs-server", Destination: "report.lifecycle", EventType: catalog.InterpretationReportGenerated, SchemaVersion: "v1", Scope: "org:1", ContentType: "application/json", OccurredAt: at.Format(time.RFC3339Nano), Payload: []byte(`{}`)}
	m, err := message.New(in)
	require.NoError(t, err)
	plan, err := transaction.NewPlan(transaction.PlanInput{Owner: "qs.mongo", Scope: in.Scope, OperationID: "report-operation", IdempotencyKey: "report/1", Branches: []transaction.BranchInput{{ID: "generated", Message: m, Wire: in.Payload, PhysicalTopic: "fixture-transaction-topic", RouteRevision: "m3-1"}}})
	require.NoError(t, err)
	ref, err := plan.Reference("generated", 1, "m3-1")
	require.NoError(t, err)
	fp := m.Fingerprint()
	standard := evidence.StandardReference{EventID: in.ID, Producer: in.Producer, Destination: in.Destination, EventType: in.EventType, SchemaVersion: in.SchemaVersion, Scope: in.Scope, ContentType: in.ContentType, OccurredAt: in.OccurredAt, Fingerprint: hex.EncodeToString(fp[:])}
	proof, err := evidence.NewTransactional(standard, ref, strings.Repeat("b", 64), at)
	require.NoError(t, err)
	return proof
}

func TestTransactionGenerationRetainsOneLifecycleVersionAndExactReference(t *testing.T) {
	at := time.Date(2026, 10, 20, 1, 2, 3, 123000000, time.UTC)
	g, err := New(1, Key{OutcomeID: 9, ReportType: policy.ReportTypeStandard, TemplateVersion: "v1"}, at.Add(-time.Minute))
	require.NoError(t, err)
	require.NoError(t, g.Begin(2, at.Add(-time.Second)))
	proof := transactionProof(t, at)
	require.NoError(t, g.SucceedWithTransactionEvidence(2, 3, at, proof))
	require.EqualValues(t, 3, g.Version())
	require.Equal(t, StatusGenerated, g.Status())
	require.Equal(t, proof.Reference.EventID, g.GeneratedEventID())
	require.Nil(t, g.GeneratedEventEvidence(), "V2 must not become Outbox proof")
	proof.TransactionReference[0] ^= 1
	getter := g.GeneratedTransactionEvidence()
	require.NoError(t, getter.Validate())
	getter.TransactionReference[0] ^= 1
	require.NoError(t, g.GeneratedTransactionEvidence().Validate())
	input := RestoreInput{ID: g.ID(), Key: g.Key(), Status: g.Status(), LatestRunID: g.LatestRunID(), ReportID: g.ReportID(), GeneratedEventID: g.GeneratedEventID(), GeneratedTransactionEvidence: g.GeneratedTransactionEvidence(), Version: g.Version(), CreatedAt: g.CreatedAt(), UpdatedAt: g.UpdatedAt()}
	copy, err := Restore(input)
	require.NoError(t, err)
	input.GeneratedTransactionEvidence.TransactionReference[0] ^= 1
	require.NoError(t, copy.GeneratedTransactionEvidence().Validate())
}

func TestTransactionGenerationRejectsMixedIdentityAndInvalidTransitionWithoutMutation(t *testing.T) {
	at := time.Date(2026, 10, 20, 1, 2, 3, 0, time.UTC)
	for _, kind := range []string{"nil", "wrong-event", "historical-origin", "wrong-time", "wrong-run"} {
		t.Run(kind, func(t *testing.T) {
			g, err := New(1, Key{OutcomeID: 9, ReportType: policy.ReportTypeStandard, TemplateVersion: "v1"}, at.Add(-time.Minute))
			require.NoError(t, err)
			require.NoError(t, g.Begin(2, at.Add(-time.Second)))
			proof, run := transactionProof(t, at), g.LatestRunID()
			switch kind {
			case "nil":
				proof = nil
			case "wrong-event":
				proof.Reference.EventType = catalog.InterpretationReportFailed
			case "historical-origin":
				proof.Origin = "backfilled_existing"
			case "wrong-time":
				proof.RecordedAt = at.Add(time.Millisecond)
			case "wrong-run":
				run = 99
			}
			require.Error(t, g.SucceedWithTransactionEvidence(run, 3, at, proof))
			require.Equal(t, StatusGenerating, g.Status())
			require.EqualValues(t, 2, g.Version())
			require.Empty(t, g.GeneratedEventID())
		})
	}
	proof := transactionProof(t, at)
	legacy, err := evidence.NewStandard(proof.Reference, proof.BusinessBindingSHA256)
	require.NoError(t, err)
	input := RestoreInput{ID: 1, Key: Key{OutcomeID: 9, ReportType: policy.ReportTypeStandard, TemplateVersion: "v1"}, Status: StatusGenerated, LatestRunID: 2, ReportID: 3, GeneratedEventID: proof.Reference.EventID, GeneratedEventEvidence: legacy, GeneratedTransactionEvidence: proof, Version: 3, CreatedAt: at, UpdatedAt: at}
	_, err = Restore(input)
	require.Error(t, err)
	input.GeneratedEventEvidence = nil
	input.GeneratedEventID = "other-event"
	_, err = Restore(input)
	require.Error(t, err)
}
