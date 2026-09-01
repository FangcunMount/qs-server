package evaluation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	domainevaluation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/evaluation"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	preparedLeaseRecoveryActor  = "system:ai-explanation-prompt-evaluation-lease-recovery"
	preparedLeaseRecoveryReason = "expired prepared execution reawakened by lease recovery"
)

type PreparedRecoveryCommitter interface {
	CommitExpiredPreparationRecovery(context.Context, meta.ID, string, time.Time, string, string, string) (*domainevaluation.PromptEvaluationRun, error)
}

type PreparedRecoveryCommitterV2 interface {
	CommitExpiredPreparationRecoveryV2(context.Context, meta.ID, string, time.Time, string) (*domainevaluation.PromptEvaluationEvidenceV2, error)
}

// PreparedLeaseRecoverer discovers only expired prepared checkpoints and
// commits a durable wake-up. It never invokes a Provider. The committer and
// aggregate recheck the exact invocation and lease expiry under optimistic
// concurrency, so a stale or dispatching candidate is skipped, not replayed.
type PreparedLeaseRecoverer struct {
	reader      domainevaluation.ExpiredPreparationReader
	committer   PreparedRecoveryCommitter
	readerV2    domainevaluation.ExpiredPreparationV2Reader
	committerV2 PreparedRecoveryCommitterV2
}

func NewPreparedLeaseRecoverer(
	reader domainevaluation.ExpiredPreparationReader,
	committer PreparedRecoveryCommitter,
	readerV2 domainevaluation.ExpiredPreparationV2Reader,
	committerV2 PreparedRecoveryCommitterV2,
) (*PreparedLeaseRecoverer, error) {
	if reader == nil || committer == nil || readerV2 == nil || committerV2 == nil {
		return nil, fmt.Errorf("AI explanation Prompt evaluation prepared lease recovery dependencies are required")
	}
	return &PreparedLeaseRecoverer{reader: reader, committer: committer, readerV2: readerV2, committerV2: committerV2}, nil
}

func (r *PreparedLeaseRecoverer) RecoverExpiredLeases(ctx context.Context, at time.Time, limit int) (int, error) {
	if r == nil || r.reader == nil || r.committer == nil || r.readerV2 == nil || r.committerV2 == nil || at.IsZero() || limit < 1 {
		return 0, fmt.Errorf("AI explanation Prompt evaluation prepared lease recovery is not configured")
	}
	v2Candidates, err := r.readerV2.ListExpiredPreparationsV2(ctx, at, limit)
	if err != nil {
		return 0, err
	}
	recovered := 0
	for _, candidate := range v2Candidates {
		requestID := preparedLeaseRecoveryRequestID(candidate.RunID, candidate.InvocationID, candidate.LeaseExpiresAt)
		_, err := r.committerV2.CommitExpiredPreparationRecoveryV2(
			ctx, candidate.RunID, candidate.InvocationID, candidate.LeaseExpiresAt, requestID,
		)
		accepted, stop := recordPreparedLeaseRecoveryResult(err)
		if accepted {
			recovered++
		}
		if stop != nil {
			return recovered, stop
		}
	}
	remaining := limit - len(v2Candidates)
	if remaining <= 0 {
		return recovered, nil
	}
	candidates, err := r.reader.ListExpiredPreparations(ctx, at, remaining)
	if err != nil {
		return recovered, err
	}
	for _, candidate := range candidates {
		requestID := preparedLeaseRecoveryRequestID(candidate.RunID, candidate.InvocationID, candidate.LeaseExpiresAt)
		_, err := r.committer.CommitExpiredPreparationRecovery(
			ctx, candidate.RunID, candidate.InvocationID, candidate.LeaseExpiresAt, requestID, preparedLeaseRecoveryActor, preparedLeaseRecoveryReason,
		)
		accepted, stop := recordPreparedLeaseRecoveryResult(err)
		if accepted {
			recovered++
		}
		if stop != nil {
			return recovered, stop
		}
	}
	return recovered, nil
}

func recordPreparedLeaseRecoveryResult(err error) (accepted bool, stop error) {
	switch {
	case err == nil:
		preparedLeaseRecoveryItemsTotal.WithLabelValues("reawakened").Inc()
		return true, nil
	case errors.Is(err, domainevaluation.ErrConflict), errors.Is(err, domainevaluation.ErrNotFound):
		preparedLeaseRecoveryItemsTotal.WithLabelValues("raced").Inc()
		return false, nil
	case errors.Is(err, domainevaluation.ErrRecoveryNotAllowed):
		preparedLeaseRecoveryItemsTotal.WithLabelValues("ineligible").Inc()
		return false, nil
	default:
		preparedLeaseRecoveryItemsTotal.WithLabelValues("error").Inc()
		return false, err
	}
}

func preparedLeaseRecoveryRequestID(runID meta.ID, invocationID string, leaseExpiresAt time.Time) string {
	digest := sha256.Sum256([]byte(runID.String() + "\x00" + invocationID + "\x00" + leaseExpiresAt.UTC().Format(time.RFC3339Nano)))
	return "auto-prepared:" + hex.EncodeToString(digest[:16])
}

var preparedLeaseRecoveryItemsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: "qs", Subsystem: "ai_explanation_prompt_evaluation_lease_recovery", Name: "items_total",
	Help: "Expired prepared Prompt evaluation checkpoints by recovery outcome.",
}, []string{"result"})

var _ interface {
	RecoverExpiredLeases(context.Context, time.Time, int) (int, error)
} = (*PreparedLeaseRecoverer)(nil)
