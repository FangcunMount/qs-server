package statistics

import (
	"context"
	"time"

	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	domainStatistics "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
)

const (
	behaviorAttributionWindow     = 30 * 24 * time.Hour
	defaultBehaviorPendingBackoff = 10 * time.Second
	maxBehaviorPendingBackoff     = 5 * time.Minute
)

type BehaviorProjectEventStatus string

const (
	BehaviorProjectEventStatusCompleted BehaviorProjectEventStatus = "completed"
	BehaviorProjectEventStatusPending   BehaviorProjectEventStatus = "pending"
)

type BehaviorProjectEventInput struct {
	EventID           string    `json:"event_id"`
	EventType         string    `json:"event_type"`
	OrgID             int64     `json:"org_id"`
	ClinicianID       uint64    `json:"clinician_id,omitempty"`
	SourceClinicianID uint64    `json:"source_clinician_id,omitempty"`
	EntryID           uint64    `json:"entry_id,omitempty"`
	TesteeID          uint64    `json:"testee_id,omitempty"`
	AnswerSheetID     uint64    `json:"answersheet_id,omitempty"`
	AssessmentID      uint64    `json:"assessment_id,omitempty"`
	ReportID          uint64    `json:"report_id,omitempty"`
	FailureReason     string    `json:"failure_reason,omitempty"`
	OccurredAt        time.Time `json:"occurred_at"`
}

type BehaviorProjectEventResult struct {
	Status BehaviorProjectEventStatus
}

type BehaviorProjectorService interface {
	ProjectBehaviorEvent(ctx context.Context, input BehaviorProjectEventInput) (BehaviorProjectEventResult, error)
	ReconcilePendingBehaviorEvents(ctx context.Context, limit int) (int, error)
}

type assessmentEpisodeProjector struct {
	uow     apptransaction.Runner
	repo    BehaviorJourneyRepository
	router  behaviorEventRouter
	pending pendingRetryQueue
}

func NewAssessmentEpisodeProjectorWithTransactionRunner(runner apptransaction.Runner, repo BehaviorJourneyRepository) BehaviorProjectorService {
	if runner == nil || repo == nil {
		return nil
	}
	journey := journeyWriter{repo: repo}
	lifecycler := episodeLifecycler{repo: repo, journey: journey}
	return &assessmentEpisodeProjector{
		uow:     runner,
		repo:    repo,
		router:  behaviorEventRouter{lifecycler: lifecycler},
		pending: pendingRetryQueue{repo: repo},
	}
}

func (p *assessmentEpisodeProjector) ProjectBehaviorEvent(ctx context.Context, input BehaviorProjectEventInput) (BehaviorProjectEventResult, error) {
	result := BehaviorProjectEventResult{Status: BehaviorProjectEventStatusCompleted}
	err := p.uow.WithinTransaction(ctx, func(txCtx context.Context) error {
		existing, err := p.repo.TryBeginAnalyticsProjectorCheckpoint(txCtx, input.EventID, input.EventType)
		if err != nil {
			return err
		}
		if existing != "" {
			if existing == domainStatistics.AnalyticsProjectorCheckpointStatusPending {
				result.Status = BehaviorProjectEventStatusPending
			}
			return nil
		}

		status, err := p.router.projectEvent(txCtx, input)
		if err != nil {
			return err
		}
		if status == BehaviorProjectEventStatusPending {
			result.Status = status
			if err := p.pending.enqueue(txCtx, input, 1, "pending_attribution"); err != nil {
				return err
			}
			return p.repo.MarkAnalyticsProjectorCheckpointStatus(txCtx, input.EventID, domainStatistics.AnalyticsProjectorCheckpointStatusPending)
		}
		return p.repo.MarkAnalyticsProjectorCheckpointStatus(txCtx, input.EventID, domainStatistics.AnalyticsProjectorCheckpointStatusCompleted)
	})
	return result, err
}

func (p *assessmentEpisodeProjector) ReconcilePendingBehaviorEvents(ctx context.Context, limit int) (int, error) {
	startedAt := time.Now()
	processed := 0
	metrics := pendingReconcileMetrics{}
	var resultErr error
	defer func() { observePendingReconcile(startedAt, metrics, resultErr) }()
	if limit <= 0 {
		limit = 100
	}
	rows, err := p.pending.listDue(ctx, limit, time.Now())
	if err != nil {
		resultErr = err
		return 0, err
	}
	for _, item := range rows {
		if item == nil {
			continue
		}
		input, err := p.pending.decode(item)
		if err != nil {
			if txErr := p.uow.WithinTransaction(ctx, func(txCtx context.Context) error {
				return p.pending.reschedule(txCtx, item.EventID, err.Error(), item.AttemptCount+1)
			}); txErr != nil {
				metrics.failed++
				resultErr = txErr
				return processed, txErr
			}
			metrics.rescheduledError++
			continue
		}

		status := BehaviorProjectEventStatusCompleted
		projectErr := p.uow.WithinTransaction(ctx, func(txCtx context.Context) error {
			var err error
			status, err = p.router.projectEvent(txCtx, input)
			if err != nil {
				return err
			}
			if status == BehaviorProjectEventStatusPending {
				return nil
			}
			if err := p.pending.delete(txCtx, input.EventID); err != nil {
				return err
			}
			return p.repo.MarkAnalyticsProjectorCheckpointStatus(txCtx, input.EventID, domainStatistics.AnalyticsProjectorCheckpointStatusCompleted)
		})
		if projectErr != nil {
			if txErr := p.uow.WithinTransaction(ctx, func(txCtx context.Context) error {
				return p.pending.reschedule(txCtx, input.EventID, projectErr.Error(), item.AttemptCount+1)
			}); txErr != nil {
				metrics.failed++
				resultErr = txErr
				return processed, txErr
			}
			metrics.rescheduledError++
			continue
		}
		if status == BehaviorProjectEventStatusPending {
			if txErr := p.uow.WithinTransaction(ctx, func(txCtx context.Context) error {
				return p.pending.reschedule(txCtx, input.EventID, "pending_attribution", item.AttemptCount+1)
			}); txErr != nil {
				metrics.failed++
				resultErr = txErr
				return processed, txErr
			}
			metrics.rescheduledPending++
			continue
		}
		processed++
		metrics.completed++
	}
	return processed, nil
}
