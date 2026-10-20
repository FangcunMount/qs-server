package run

import (
	"encoding/hex"
	"strings"
	"testing"
	"time"

	catalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
	"github.com/FangcunMount/reliable-messaging/message"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/stretchr/testify/require"
)

func failedProof(t *testing.T, at time.Time) *evidence.EventEvidenceV2 {
	t.Helper()
	in := message.Input{ID: "failed-event", Producer: "qs-server", Destination: "reports.lifecycle", EventType: catalog.InterpretationReportFailed, SchemaVersion: "v1", Scope: "org:1", ContentType: "application/json", OccurredAt: at.Format(time.RFC3339Nano), Payload: []byte(`{}`)}
	m, err := message.New(in)
	require.NoError(t, err)
	p, err := transaction.NewPlan(transaction.PlanInput{Owner: "qs.mongo", Scope: in.Scope, OperationID: "failure-operation", IdempotencyKey: "report-failed/1", Branches: []transaction.BranchInput{{ID: "failed", Message: m, Wire: in.Payload, PhysicalTopic: "fixture-transaction-topic", RouteRevision: "m3-1"}}})
	require.NoError(t, err)
	ref, err := p.Reference("failed", 1, "m3-1")
	require.NoError(t, err)
	fp := m.Fingerprint()
	standard := evidence.StandardReference{EventID: in.ID, Producer: in.Producer, Destination: in.Destination, EventType: in.EventType, SchemaVersion: in.SchemaVersion, Scope: in.Scope, ContentType: in.ContentType, OccurredAt: in.OccurredAt, Fingerprint: hex.EncodeToString(fp[:])}
	proof, err := evidence.NewTransactional(standard, ref, strings.Repeat("b", 64), at)
	require.NoError(t, err)
	return proof
}

func TestFailureTransactionDoesNotAuthorizeOrRewriteFutureBusinessRetry(t *testing.T) {
	at := time.Date(2026, 10, 20, 1, 2, 3, 123000000, time.UTC)
	r, err := NewPending(1, 2, 1)
	require.NoError(t, err)
	require.NoError(t, r.Start(at.Add(-time.Second), "original-trace"))
	require.NoError(t, r.Fail(at, Failure{Kind: FailureKindBuild, Code: "failed", SafeMessage: "failed", Retryable: false}))
	decision := r.RetryDecision()
	proof := failedProof(t, at)
	require.NoError(t, r.AttachFailureTransactionEvidence(proof))
	require.Equal(t, decision, r.RetryDecision())
	require.Equal(t, 1, r.Attempt())
	require.Nil(t, r.RetryEventEvidence())
	first := r.FailedTransactionEvidence()
	require.NoError(t, r.AuthorizeOneRetry(retrygovernance.AttemptOriginForce, "operator-request", "future-retry-event", at.Add(time.Minute)))
	require.Equal(t, retrygovernance.DispositionAutomatic, r.RetryDecision().Disposition)
	require.Equal(t, first, r.FailedTransactionEvidence())
	proof.TransactionReference[0] ^= 1
	require.NoError(t, r.FailedTransactionEvidence().Validate())
	different := first.Clone()
	different.BusinessBindingSHA256 = strings.Repeat("c", 64)
	require.Error(t, r.AttachFailureTransactionEvidence(different))
	require.Equal(t, first, r.FailedTransactionEvidence())
	got, err := Restore(RestoreInput{ID: r.ID(), GenerationID: r.GenerationID(), Attempt: r.Attempt(), Status: r.Status(), Failure: r.Failure(), TraceID: r.TraceID(), StartedAt: r.StartedAt(), FinishedAt: r.FinishedAt(), Origin: r.Origin(), RetryDecision: r.RetryDecision(), FailedTransactionEvidence: r.FailedTransactionEvidence()})
	require.NoError(t, err)
	require.Equal(t, first, got.FailedTransactionEvidence())
	require.Equal(t, r.RetryDecision(), got.RetryDecision())
}

func TestFailureTransactionRejectsNonterminalWrongTypeAndWrongClock(t *testing.T) {
	at := time.Date(2026, 10, 20, 1, 2, 3, 0, time.UTC)
	r, err := NewPending(1, 2, 1)
	require.NoError(t, err)
	require.NoError(t, r.Start(at.Add(-time.Second), "trace"))
	require.Error(t, r.AttachFailureTransactionEvidence(failedProof(t, at)))
	require.NoError(t, r.Fail(at, Failure{Kind: FailureKindBuild, Code: "failed", SafeMessage: "failed", Retryable: true}))
	wrong := failedProof(t, at)
	wrong.Reference.EventType = catalog.InterpretationReportGenerated
	require.Error(t, r.AttachFailureTransactionEvidence(wrong))
	wrong = failedProof(t, at)
	wrong.RecordedAt = at.Add(time.Millisecond)
	require.Error(t, r.AttachFailureTransactionEvidence(wrong))
	require.Nil(t, r.FailedTransactionEvidence())
}
