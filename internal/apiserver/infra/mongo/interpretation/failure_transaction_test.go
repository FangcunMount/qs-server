package interpretation

import (
	"testing"
	"time"

	run "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/run"
	catalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
)

func TestFailedRunBSONKeepsOriginalFailureReferenceAndRejectsOutsideGuard(t *testing.T) {
	proof := transactionLifecycleProof(t, catalog.InterpretationReportFailed)
	r, err := run.NewPending(2, 1, 1)
	require.NoError(t, err)
	require.NoError(t, r.Start(proof.RecordedAt.Add(-time.Second), "original-claim"))
	require.NoError(t, r.Fail(proof.RecordedAt, run.Failure{Kind: run.FailureKindBuild, Code: "failed", SafeMessage: "failed", Retryable: false}))
	require.NoError(t, r.AttachFailureTransactionEvidence(proof))
	mapper := NewLifecycleMapper()
	raw, err := bson.Marshal(mapper.RunToPO(r))
	require.NoError(t, err)
	var po InterpretationRunPO
	require.NoError(t, bson.Unmarshal(raw, &po))
	got, err := mapper.RunToDomain(&po)
	require.NoError(t, err)
	require.Equal(t, proof.TransactionReference, got.FailedTransactionEvidence().TransactionReference)
	require.Equal(t, r.RetryDecision(), got.RetryDecision())
	require.Nil(t, got.RetryEventEvidence())
	po.FailedTransactionEvidence.TransactionReference[0] ^= 1
	require.NoError(t, got.FailedTransactionEvidence().Validate())
	repo := &RunRepository{mapper: mapper}
	require.ErrorIs(t, repo.Create(t.Context(), got), transaction.ErrTransactionRequired)
	require.ErrorIs(t, repo.Save(t.Context(), got), transaction.ErrTransactionRequired)
	po = *mapper.RunToPO(r)
	ref, err := transaction.DecodeReference(po.FailedTransactionEvidence.TransactionReference)
	require.NoError(t, err)
	ref.Owner = "qs.mysql"
	po.FailedTransactionEvidence.TransactionReference, err = transaction.EncodeReference(ref)
	require.NoError(t, err)
	_, err = mapper.RunToDomain(&po)
	require.ErrorIs(t, err, transaction.ErrBinding)
}
