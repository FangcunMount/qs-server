package interpretation

import (
	"bytes"
	"testing"
	"time"

	run "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/run"
	catalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
)

func runningTerminalFixture(t *testing.T) *run.InterpretationRun {
	t.Helper()
	at := time.Date(2026, 10, 20, 1, 2, 2, 123456789, time.UTC)
	r, err := run.NewPending(2, 1, 1)
	require.NoError(t, err)
	require.NoError(t, r.StartWithLease(at, "first-worker", at.Add(time.Minute)))
	return r
}

func terminalCopy(t *testing.T, r *run.InterpretationRun) *run.InterpretationRun {
	t.Helper()
	mapper := NewLifecycleMapper()
	copy, err := mapper.RunToDomain(mapper.RunToPO(r))
	require.NoError(t, err)
	return copy
}

func TestFrozenTerminalCapturesExactClaimWithoutCallerAliasing(t *testing.T) {
	before := runningTerminalFixture(t)
	after := terminalCopy(t, before)
	proof := transactionLifecycleProof(t, catalog.InterpretationReportGenerated)
	require.NoError(t, after.Succeed(proof.RecordedAt))
	f, err := FreezeRunTerminal(before, after, proof)
	require.NoError(t, err)
	filter, update := bytes.Clone(f.filter), bytes.Clone(f.update)
	// Reclaim the caller's old aggregate after freezing. Neither the filter,
	// terminal clocks nor the stored reference may follow these mutations.
	reclaimAt := before.LeaseExpiresAt().Add(time.Second)
	require.NoError(t, before.ReclaimExpiredLease(reclaimAt, "next-worker", reclaimAt.Add(time.Minute)))
	proof.TransactionReference[0] ^= 1
	require.Equal(t, filter, []byte(f.filter))
	require.Equal(t, update, []byte(f.update))
	require.NoError(t, f.proof.Validate())
	var decoded bson.M
	require.NoError(t, bson.Unmarshal(f.filter, &decoded))
	require.Equal(t, "first-worker", decoded["trace_id"])
	require.Contains(t, decoded, "lease_expires_at")
	require.Contains(t, decoded, "started_at")
	require.Contains(t, decoded, "recovery_count")
	var body struct {
		Set struct {
			UpdatedAt time.Time `bson:"updated_at"`
		} `bson:"$set"`
	}
	require.NoError(t, bson.Unmarshal(f.update, &body))
	require.True(t, after.FinishedAt().UTC().Truncate(time.Millisecond).Equal(body.Set.UpdatedAt))
	// A correct-looking value outside the exact original guard still cannot
	// issue any Mongo operation (this fixture has no database connection).
	require.ErrorIs(t, f.Write(t.Context(), &RunRepository{}), transaction.ErrTransactionRequired)
}

func TestFrozenTerminalRefusesDifferentClaimAndFailureProof(t *testing.T) {
	for _, kind := range []string{"trace", "recovery", "wrong-kind", "missing-failure-proof", "different-failure-proof"} {
		t.Run(kind, func(t *testing.T) {
			before := runningTerminalFixture(t)
			after := terminalCopy(t, before)
			proof := transactionLifecycleProof(t, catalog.InterpretationReportGenerated)
			switch kind {
			case "trace", "recovery":
				at := after.LeaseExpiresAt().Add(time.Second)
				trace := after.TraceID()
				if kind == "trace" {
					trace = "other-worker"
				}
				require.NoError(t, after.ReclaimExpiredLease(at, trace, at.Add(time.Minute)))
				require.NoError(t, after.Succeed(proof.RecordedAt))
			case "wrong-kind":
				proof = transactionLifecycleProof(t, catalog.InterpretationReportFailed)
				require.NoError(t, after.Succeed(proof.RecordedAt))
			default:
				proof = transactionLifecycleProof(t, catalog.InterpretationReportFailed)
				require.NoError(t, after.Fail(proof.RecordedAt, run.Failure{Kind: run.FailureKindBuild, Code: "failed", SafeMessage: "failed", Retryable: false}))
				if kind == "different-failure-proof" {
					attached := proof.Clone()
					attached.BusinessBindingSHA256 = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
					require.NoError(t, after.AttachFailureTransactionEvidence(attached))
				}
			}
			_, err := FreezeRunTerminal(before, after, proof)
			require.Error(t, err)
		})
	}
}

func TestFrozenFailedTerminalPreservesFutureRetryResponsibility(t *testing.T) {
	before := runningTerminalFixture(t)
	after := terminalCopy(t, before)
	proof := transactionLifecycleProof(t, catalog.InterpretationReportFailed)
	require.NoError(t, after.Fail(proof.RecordedAt, run.Failure{Kind: run.FailureKindBuild, Code: "failed", SafeMessage: "failed", Retryable: true}))
	require.NoError(t, after.AttachFailureTransactionEvidence(proof))
	f, err := FreezeRunTerminal(before, after, proof)
	require.NoError(t, err)
	var document struct {
		Set InterpretationRunPO `bson:"$set"`
	}
	require.NoError(t, bson.Unmarshal(f.update, &document))
	require.NotNil(t, document.Set.NextAttemptAt)
	require.True(t, after.RetryDecision().NextAttemptAt.UTC().Truncate(time.Millisecond).Equal(*document.Set.NextAttemptAt))
	require.Equal(t, 1, before.Attempt())
	require.Nil(t, document.Set.RetryEventEvidence)
	require.Equal(t, proof.TransactionReference, document.Set.FailedTransactionEvidence.TransactionReference)
}
