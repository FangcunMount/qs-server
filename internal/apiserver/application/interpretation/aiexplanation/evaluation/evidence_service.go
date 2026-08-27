package evaluation

import (
	"context"
	"fmt"
	"strings"
	"time"

	domainevaluation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/evaluation"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// EvidenceService is the application boundary for accumulating an immutable
// PromptEvaluationRun. Provider execution and human review transports remain
// replaceable; every mutation is protected by aggregate-version CAS.
type EvidenceService struct {
	repository domainevaluation.Repository
	newID      func() meta.ID
	now        func() time.Time
}

func NewEvidenceService(repository domainevaluation.Repository, newID func() meta.ID, now func() time.Time) (*EvidenceService, error) {
	if repository == nil {
		return nil, fmt.Errorf("AI explanation Prompt evaluation repository is required")
	}
	if newID == nil {
		newID = meta.New
	}
	if now == nil {
		now = time.Now
	}
	return &EvidenceService{repository: repository, newID: newID, now: now}, nil
}

func (s *EvidenceService) Start(ctx context.Context, release domainevaluation.ReleaseIdentity) (*domainevaluation.PromptEvaluationRun, error) {
	if s == nil {
		return nil, fmt.Errorf("AI explanation Prompt evaluation service is required")
	}
	runRecord, err := domainevaluation.New(s.newID(), release, s.now())
	if err != nil {
		return nil, err
	}
	if err := s.repository.Create(ctx, runRecord); err != nil {
		return nil, err
	}
	return runRecord, nil
}

func (s *EvidenceService) RecordAttempt(ctx context.Context, runID meta.ID, attempt domainevaluation.AttemptRecord) (*domainevaluation.PromptEvaluationRun, error) {
	return s.mutate(ctx, runID, func(runRecord *domainevaluation.PromptEvaluationRun) error {
		return runRecord.AddAttempt(attempt)
	})
}

type ClaimAttemptCommand struct {
	CaseID         string
	Attempt        int
	Owner          string
	InvocationID   string
	ClaimedAt      time.Time
	LeaseExpiresAt time.Time
}

func (s *EvidenceService) ClaimAttempt(ctx context.Context, runID meta.ID, command ClaimAttemptCommand) (*domainevaluation.PromptEvaluationRun, error) {
	return s.mutate(ctx, runID, func(runRecord *domainevaluation.PromptEvaluationRun) error {
		return runRecord.BeginAttemptExecution(domainevaluation.AttemptExecution{
			CaseID: command.CaseID, Attempt: command.Attempt, Owner: command.Owner, InvocationID: command.InvocationID,
			Phase: domainevaluation.AttemptExecutionPrepared, ClaimedAt: command.ClaimedAt, LeaseExpiresAt: command.LeaseExpiresAt,
		})
	})
}

func (s *EvidenceService) MarkAttemptDispatching(ctx context.Context, runID meta.ID, owner string, at time.Time) (*domainevaluation.PromptEvaluationRun, error) {
	return s.mutate(ctx, runID, func(runRecord *domainevaluation.PromptEvaluationRun) error {
		return runRecord.MarkAttemptDispatching(owner, at)
	})
}

func (s *EvidenceService) CompleteAttempt(ctx context.Context, runID meta.ID, owner string, attempt domainevaluation.AttemptRecord) (*domainevaluation.PromptEvaluationRun, error) {
	return s.mutate(ctx, runID, func(runRecord *domainevaluation.PromptEvaluationRun) error {
		return runRecord.CompleteAttemptExecution(owner, attempt)
	})
}

func (s *EvidenceService) ReleaseExpiredPreparation(ctx context.Context, runID meta.ID, at time.Time) (*domainevaluation.PromptEvaluationRun, error) {
	return s.mutate(ctx, runID, func(runRecord *domainevaluation.PromptEvaluationRun) error {
		return runRecord.ReleaseExpiredPreparation(at)
	})
}

func (s *EvidenceService) CloseCollection(ctx context.Context, runID meta.ID) (*domainevaluation.PromptEvaluationRun, error) {
	return s.mutate(ctx, runID, func(runRecord *domainevaluation.PromptEvaluationRun) error {
		return runRecord.CloseCollection(s.now())
	})
}

func (s *EvidenceService) Cancel(ctx context.Context, runID meta.ID, actor, reason string) (*domainevaluation.PromptEvaluationRun, error) {
	return s.mutate(ctx, runID, func(runRecord *domainevaluation.PromptEvaluationRun) error {
		return runRecord.Cancel(actor, reason, s.now().UTC())
	})
}

type HumanReviewCommand struct {
	CaseID   string
	Attempt  int
	Role     domainevaluation.ReviewRole
	Reviewer string
	Decision domainevaluation.ReviewDecision
	Reason   string
}

func (s *EvidenceService) RecordHumanReview(ctx context.Context, runID meta.ID, command HumanReviewCommand) (*domainevaluation.PromptEvaluationRun, error) {
	return s.mutate(ctx, runID, func(runRecord *domainevaluation.PromptEvaluationRun) error {
		return runRecord.AddHumanReview(domainevaluation.HumanReview{
			CaseID: command.CaseID, Attempt: command.Attempt, Role: command.Role, Reviewer: command.Reviewer,
			Decision: command.Decision, ReviewedAt: s.now(), Reason: command.Reason,
		})
	})
}

func (s *EvidenceService) Finalize(ctx context.Context, runID meta.ID, actor, reason string) (*domainevaluation.PromptEvaluationRun, error) {
	if strings.TrimSpace(actor) == "" || strings.TrimSpace(reason) == "" {
		return nil, fmt.Errorf("AI explanation Prompt evaluation finalization audit is required")
	}
	return s.mutate(ctx, runID, func(runRecord *domainevaluation.PromptEvaluationRun) error {
		return runRecord.Finalize(actor, reason, s.now())
	})
}

func (s *EvidenceService) Find(ctx context.Context, runID meta.ID) (*domainevaluation.PromptEvaluationRun, error) {
	if s == nil || runID.IsZero() {
		return nil, fmt.Errorf("AI explanation Prompt evaluation run id is required")
	}
	return s.repository.FindByID(ctx, runID)
}

func (s *EvidenceService) mutate(ctx context.Context, runID meta.ID, mutation func(*domainevaluation.PromptEvaluationRun) error) (*domainevaluation.PromptEvaluationRun, error) {
	if s == nil || runID.IsZero() || mutation == nil {
		return nil, fmt.Errorf("AI explanation Prompt evaluation mutation is invalid")
	}
	runRecord, err := s.repository.FindByID(ctx, runID)
	if err != nil {
		return nil, err
	}
	expectedVersion := runRecord.Version()
	if err := mutation(runRecord); err != nil {
		return nil, err
	}
	if err := s.repository.Save(ctx, runRecord, expectedVersion); err != nil {
		return nil, err
	}
	return runRecord, nil
}
