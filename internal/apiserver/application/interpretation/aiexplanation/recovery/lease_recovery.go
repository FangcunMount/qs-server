package recovery

import (
	"context"
	"errors"
	"fmt"
	"time"

	aiexplanationevents "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/events"
	domaingeneration "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/generation"
	domainrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/run"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type LeaseRecoveryCommitter interface {
	CommitLeaseRecoveryWakeup(context.Context, *domaingeneration.AIExplanationGeneration, *domainrun.AIExplanationRun, domainrun.RecoveryWakeup) (bool, error)
}

// LeaseRecoverer scans bounded expired leases and commits deterministic
// durable wake-ups. It never calls the Provider and never creates a new
// business attempt; Worker execution rechecks the exact Run and lease.
type LeaseRecoverer struct {
	reader      domainrun.ExpiredLeaseReader
	generations domaingeneration.Repository
	runs        domainrun.Repository
	committer   LeaseRecoveryCommitter
}

func NewLeaseRecoverer(
	reader domainrun.ExpiredLeaseReader,
	generations domaingeneration.Repository,
	runs domainrun.Repository,
	committer LeaseRecoveryCommitter,
) (*LeaseRecoverer, error) {
	if reader == nil || generations == nil || runs == nil || committer == nil {
		return nil, fmt.Errorf("AI explanation participant lease recovery dependencies are required")
	}
	return &LeaseRecoverer{reader: reader, generations: generations, runs: runs, committer: committer}, nil
}

func (r *LeaseRecoverer) RecoverExpiredLeases(ctx context.Context, at time.Time, limit int) (int, error) {
	if r == nil || r.reader == nil || r.generations == nil || r.runs == nil || r.committer == nil || at.IsZero() || limit < 1 {
		return 0, fmt.Errorf("AI explanation participant lease recovery is not configured")
	}
	leases, err := r.reader.ListExpiredLeases(ctx, at, limit)
	if err != nil {
		return 0, err
	}
	scheduled := 0
	for _, lease := range leases {
		generationRecord, loadErr := r.generations.FindByID(ctx, lease.GenerationID)
		if loadErr != nil {
			if errors.Is(loadErr, domaingeneration.ErrNotFound) {
				observeParticipantLeaseRecovery(lease.InvocationPhase, "raced")
				continue
			}
			observeParticipantLeaseRecovery(lease.InvocationPhase, "error")
			return scheduled, loadErr
		}
		runRecord, loadErr := r.runs.FindByID(ctx, lease.RunID)
		if loadErr != nil {
			if errors.Is(loadErr, domainrun.ErrNotFound) {
				observeParticipantLeaseRecovery(lease.InvocationPhase, "raced")
				continue
			}
			observeParticipantLeaseRecovery(lease.InvocationPhase, "error")
			return scheduled, loadErr
		}
		wakeup := domainrun.RecoveryWakeup{
			EventID:                aiexplanationevents.LeaseRecoveryEventID(generationRecord.ID().String(), runRecord.ID().String(), lease.LeaseExpiredAt, lease.InvocationPhase),
			ExpectedLeaseExpiresAt: lease.LeaseExpiredAt, InvocationPhase: lease.InvocationPhase, RequestedAt: at,
		}
		created, commitErr := r.committer.CommitLeaseRecoveryWakeup(ctx, generationRecord, runRecord, wakeup)
		switch {
		case commitErr == nil && created:
			observeParticipantLeaseRecovery(lease.InvocationPhase, "scheduled")
			scheduled++
		case commitErr == nil:
			observeParticipantLeaseRecovery(lease.InvocationPhase, "existing")
		case errors.Is(commitErr, domainrun.ErrRecoveryNotAllowed), errors.Is(commitErr, domainrun.ErrConflict):
			observeParticipantLeaseRecovery(lease.InvocationPhase, "raced")
		default:
			observeParticipantLeaseRecovery(lease.InvocationPhase, "error")
			return scheduled, commitErr
		}
	}
	return scheduled, nil
}

func observeParticipantLeaseRecovery(phase domainrun.InvocationPhase, result string) {
	participantLeaseRecoveryItemsTotal.WithLabelValues(string(phase), result).Inc()
}

var participantLeaseRecoveryItemsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: "qs", Subsystem: "ai_explanation_participant_lease_recovery", Name: "items_total",
	Help: "Expired Participant AI explanation Run leases by invocation phase and wake-up result.",
}, []string{"invocation_phase", "result"})

var _ interface {
	RecoverExpiredLeases(context.Context, time.Time, int) (int, error)
} = (*LeaseRecoverer)(nil)
