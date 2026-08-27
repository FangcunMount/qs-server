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

// PreparedLeaseRecoverer discovers only expired prepared checkpoints and
// commits a durable wake-up. It never invokes a Provider. The committer and
// aggregate recheck the exact invocation and lease expiry under optimistic
// concurrency, so a stale or dispatching candidate is skipped, not replayed.
type PreparedLeaseRecoverer struct {
	reader    domainevaluation.ExpiredPreparationReader
	committer PreparedRecoveryCommitter
}

func NewPreparedLeaseRecoverer(reader domainevaluation.ExpiredPreparationReader, committer PreparedRecoveryCommitter) (*PreparedLeaseRecoverer, error) {
	if reader == nil || committer == nil {
		return nil, fmt.Errorf("AI explanation Prompt evaluation prepared lease recovery dependencies are required")
	}
	return &PreparedLeaseRecoverer{reader: reader, committer: committer}, nil
}

func (r *PreparedLeaseRecoverer) RecoverExpiredLeases(ctx context.Context, at time.Time, limit int) (int, error) {
	if r == nil || r.reader == nil || r.committer == nil || at.IsZero() || limit < 1 {
		return 0, fmt.Errorf("AI explanation Prompt evaluation prepared lease recovery is not configured")
	}
	candidates, err := r.reader.ListExpiredPreparations(ctx, at, limit)
	if err != nil {
		return 0, err
	}
	recovered := 0
	for _, candidate := range candidates {
		requestID := preparedLeaseRecoveryRequestID(candidate.RunID, candidate.InvocationID, candidate.LeaseExpiresAt)
		_, err := r.committer.CommitExpiredPreparationRecovery(
			ctx, candidate.RunID, candidate.InvocationID, candidate.LeaseExpiresAt, requestID, preparedLeaseRecoveryActor, preparedLeaseRecoveryReason,
		)
		switch {
		case err == nil:
			preparedLeaseRecoveryItemsTotal.WithLabelValues("reawakened").Inc()
			recovered++
		case errors.Is(err, domainevaluation.ErrConflict), errors.Is(err, domainevaluation.ErrNotFound):
			preparedLeaseRecoveryItemsTotal.WithLabelValues("raced").Inc()
		case errors.Is(err, domainevaluation.ErrRecoveryNotAllowed):
			preparedLeaseRecoveryItemsTotal.WithLabelValues("ineligible").Inc()
		default:
			preparedLeaseRecoveryItemsTotal.WithLabelValues("error").Inc()
			return recovered, err
		}
	}
	return recovered, nil
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
