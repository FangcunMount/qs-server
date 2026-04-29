package statistics

import (
	"context"
	"time"

	assessmentEntryApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/assessmententry"
	clinicianApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/clinician"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	domainStatistics "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	"github.com/FangcunMount/qs-server/pkg/event"
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

type behaviorEventStager struct {
	outboxStore behaviorEventOutboxStore
}

type behaviorEventOutboxStore interface {
	Stage(ctx context.Context, events ...event.DomainEvent) error
}

func NewBehaviorEventStager(outboxStore behaviorEventOutboxStore) interface {
	assessmentEntryApp.BehaviorEventStager
	clinicianApp.BehaviorEventStager
} {
	if outboxStore == nil {
		return nil
	}
	return &behaviorEventStager{outboxStore: outboxStore}
}

func (s *behaviorEventStager) stage(ctx context.Context, evt event.DomainEvent) error {
	if s == nil || s.outboxStore == nil || evt == nil {
		return nil
	}
	return s.outboxStore.Stage(ctx, evt)
}

func (s *behaviorEventStager) StageEntryOpened(ctx context.Context, orgID int64, clinicianID, entryID uint64, occurredAt time.Time) error {
	return s.stage(ctx, domainStatistics.NewFootprintEntryOpenedEvent(orgID, clinicianID, entryID, occurredAt))
}

func (s *behaviorEventStager) StageIntakeConfirmed(ctx context.Context, orgID int64, clinicianID, entryID, testeeID uint64, occurredAt time.Time) error {
	return s.stage(ctx, domainStatistics.NewFootprintIntakeConfirmedEvent(orgID, clinicianID, entryID, testeeID, occurredAt))
}

func (s *behaviorEventStager) StageTesteeProfileCreated(ctx context.Context, orgID int64, clinicianID, entryID, testeeID uint64, occurredAt time.Time) error {
	return s.stage(ctx, domainStatistics.NewFootprintTesteeProfileCreatedEvent(orgID, clinicianID, entryID, testeeID, occurredAt))
}

func (s *behaviorEventStager) StageCareRelationshipEstablished(ctx context.Context, orgID int64, clinicianID, entryID, testeeID uint64, occurredAt time.Time) error {
	return s.stage(ctx, domainStatistics.NewFootprintCareRelationshipEstablishedEvent(orgID, clinicianID, entryID, testeeID, occurredAt))
}

func (s *behaviorEventStager) StageCareRelationshipTransferred(ctx context.Context, orgID int64, fromClinicianID, toClinicianID, testeeID uint64, occurredAt time.Time) error {
	return s.stage(ctx, domainStatistics.NewFootprintCareRelationshipTransferredEvent(orgID, fromClinicianID, toClinicianID, testeeID, occurredAt))
}

type assessmentEpisodeProjector struct {
	uow     apptransaction.Runner
	repo    BehaviorProjectionRepository
	router  behaviorEventRouter
	pending pendingRetryQueue
}

func NewAssessmentEpisodeProjectorWithTransactionRunner(runner apptransaction.Runner, repo BehaviorProjectionRepository) BehaviorProjectorService {
	if runner == nil || repo == nil {
		return nil
	}
	projection := projectionWriter{repo: repo}
	lifecycler := episodeLifecycler{repo: repo, projection: projection}
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
	if limit <= 0 {
		limit = 100
	}
	rows, err := p.pending.listDue(ctx, limit, time.Now())
	if err != nil {
		return 0, err
	}
	processed := 0
	for _, item := range rows {
		if item == nil {
			continue
		}
		input, err := p.pending.decode(item)
		if err != nil {
			if txErr := p.uow.WithinTransaction(ctx, func(txCtx context.Context) error {
				return p.pending.reschedule(txCtx, item.EventID, err.Error(), item.AttemptCount+1)
			}); txErr != nil {
				return processed, txErr
			}
			continue
		}

		err = p.uow.WithinTransaction(ctx, func(txCtx context.Context) error {
			status, err := p.router.projectEvent(txCtx, input)
			if err != nil {
				return p.pending.reschedule(txCtx, input.EventID, err.Error(), item.AttemptCount+1)
			}
			if status == BehaviorProjectEventStatusPending {
				return p.pending.reschedule(txCtx, input.EventID, "pending_attribution", item.AttemptCount+1)
			}
			if err := p.pending.delete(txCtx, input.EventID); err != nil {
				return err
			}
			return p.repo.MarkAnalyticsProjectorCheckpointStatus(txCtx, input.EventID, domainStatistics.AnalyticsProjectorCheckpointStatusCompleted)
		})
		if err != nil {
			return processed, err
		}
		processed++
	}
	return processed, nil
}
