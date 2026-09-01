package evaluation

import (
	"context"
	"fmt"
	"strings"
	"time"

	domainevaluation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/evaluation"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// EvidenceV2Service is the CAS application boundary for the lightweight Slot
// model. Creation and wake-up staging remain in the durable committer so a Run
// can never be created without its frozen capacity reservation.
type EvidenceV2Service struct {
	repository domainevaluation.EvidenceV2Repository
}

func NewEvidenceV2Service(repository domainevaluation.EvidenceV2Repository) (*EvidenceV2Service, error) {
	if repository == nil {
		return nil, fmt.Errorf("AI explanation Prompt evaluation evidence v2 repository is required")
	}
	return &EvidenceV2Service{repository: repository}, nil
}

func (s *EvidenceV2Service) Find(ctx context.Context, runID meta.ID) (*domainevaluation.PromptEvaluationEvidenceV2, error) {
	if s == nil || runID.IsZero() {
		return nil, fmt.Errorf("AI explanation Prompt evaluation evidence v2 Run id is required")
	}
	return s.repository.FindEvidenceV2ByID(ctx, runID)
}

func (s *EvidenceV2Service) NextAction(ctx context.Context, runID meta.ID) (domainevaluation.EvidenceNextAction, error) {
	value, err := s.Find(ctx, runID)
	if err != nil {
		return domainevaluation.EvidenceNextAction{}, err
	}
	return value.NextAction()
}

func (s *EvidenceV2Service) CompletePreflight(ctx context.Context, runID meta.ID, value domainevaluation.PreflightCaseEvidence) (*domainevaluation.PromptEvaluationEvidenceV2, error) {
	return s.mutate(ctx, runID, func(evidence *domainevaluation.PromptEvaluationEvidenceV2) error {
		return evidence.CompletePreflight(value)
	})
}

type ClaimEvidenceV2ExecutionCommand struct {
	ExecutionID    string
	Owner          string
	InvocationID   string
	ClaimedAt      time.Time
	LeaseExpiresAt time.Time
}

func (s *EvidenceV2Service) ClaimNextExecution(ctx context.Context, runID meta.ID, command ClaimEvidenceV2ExecutionCommand) (*domainevaluation.PromptEvaluationEvidenceV2, error) {
	return s.mutate(ctx, runID, func(evidence *domainevaluation.PromptEvaluationEvidenceV2) error {
		action, err := evidence.NextAction()
		if err != nil {
			return err
		}
		kind := domainevaluation.EvidenceExecutionGeneration
		if action.Kind == domainevaluation.EvidenceNextActionSemantic {
			kind = domainevaluation.EvidenceExecutionSemantic
		} else if action.Kind != domainevaluation.EvidenceNextActionGeneration {
			return fmt.Errorf("AI explanation Prompt evaluation v2 has no executable Provider action")
		}
		return evidence.BeginNextExecution(domainevaluation.EvidenceExecutionCheckpoint{
			ID: strings.TrimSpace(command.ExecutionID), Kind: kind,
			CaseID: action.CaseID, SlotOrdinal: action.SlotOrdinal, CandidateID: action.CandidateID,
			ExecutionOrdinal: action.ExecutionOrdinal, Owner: strings.TrimSpace(command.Owner),
			InvocationID: strings.TrimSpace(command.InvocationID), Phase: domainevaluation.AttemptExecutionPrepared,
			ClaimedAt: command.ClaimedAt, LeaseExpiresAt: command.LeaseExpiresAt,
		})
	})
}

func (s *EvidenceV2Service) MarkExecutionDispatching(ctx context.Context, runID meta.ID, owner string, at time.Time) (*domainevaluation.PromptEvaluationEvidenceV2, error) {
	return s.mutate(ctx, runID, func(evidence *domainevaluation.PromptEvaluationEvidenceV2) error {
		return evidence.MarkExecutionDispatching(owner, at)
	})
}

func (s *EvidenceV2Service) ReleaseExpiredPreparation(ctx context.Context, runID meta.ID, at time.Time) (*domainevaluation.PromptEvaluationEvidenceV2, error) {
	return s.mutate(ctx, runID, func(evidence *domainevaluation.PromptEvaluationEvidenceV2) error {
		return evidence.ReleaseExpiredPreparation(at)
	})
}

type CompleteGenerationV2Command struct {
	Owner       string
	CandidateID string
	Assertions  []domainevaluation.AssertionReceipt
	Execution   domainevaluation.CandidateGenerationExecution
}

func (s *EvidenceV2Service) CompleteGeneration(ctx context.Context, runID meta.ID, command CompleteGenerationV2Command) (*domainevaluation.PromptEvaluationEvidenceV2, error) {
	return s.mutate(ctx, runID, func(evidence *domainevaluation.PromptEvaluationEvidenceV2) error {
		return evidence.CompleteGenerationExecution(command.Owner, command.CandidateID, command.Assertions, command.Execution)
	})
}

func (s *EvidenceV2Service) CompleteSemantic(ctx context.Context, runID meta.ID, owner string, execution domainevaluation.SemanticEvaluationExecution) (*domainevaluation.PromptEvaluationEvidenceV2, error) {
	return s.mutate(ctx, runID, func(evidence *domainevaluation.PromptEvaluationEvidenceV2) error {
		return evidence.CompleteSemanticExecution(owner, execution)
	})
}

func (s *EvidenceV2Service) ResolveResultUnknown(ctx context.Context, runID meta.ID, value domainevaluation.ResultUnknownResolution) (*domainevaluation.PromptEvaluationEvidenceV2, error) {
	return s.mutate(ctx, runID, func(evidence *domainevaluation.PromptEvaluationEvidenceV2) error {
		return evidence.ResolveResultUnknown(value)
	})
}

func (s *EvidenceV2Service) RecordHumanReview(ctx context.Context, runID meta.ID, value domainevaluation.CandidateHumanReview) (*domainevaluation.PromptEvaluationEvidenceV2, error) {
	return s.RecordHumanReviews(ctx, runID, []domainevaluation.CandidateHumanReview{value})
}

func (s *EvidenceV2Service) RecordHumanReviews(ctx context.Context, runID meta.ID, values []domainevaluation.CandidateHumanReview) (*domainevaluation.PromptEvaluationEvidenceV2, error) {
	return s.mutate(ctx, runID, func(evidence *domainevaluation.PromptEvaluationEvidenceV2) error {
		return evidence.AddHumanReviews(values)
	})
}

func (s *EvidenceV2Service) Finalize(ctx context.Context, runID meta.ID, actor, causeCode string, at time.Time) (*domainevaluation.PromptEvaluationEvidenceV2, error) {
	actor, causeCode = strings.TrimSpace(actor), strings.TrimSpace(causeCode)
	if actor == "" || causeCode == "" || at.IsZero() {
		return nil, fmt.Errorf("AI explanation Prompt evaluation v2 finalization audit is required")
	}
	return s.mutate(ctx, runID, func(evidence *domainevaluation.PromptEvaluationEvidenceV2) error {
		return evidence.Finalize(actor, causeCode, at)
	})
}

func (s *EvidenceV2Service) mutate(ctx context.Context, runID meta.ID, mutation func(*domainevaluation.PromptEvaluationEvidenceV2) error) (*domainevaluation.PromptEvaluationEvidenceV2, error) {
	if s == nil || runID.IsZero() || mutation == nil {
		return nil, fmt.Errorf("AI explanation Prompt evaluation evidence v2 mutation is invalid")
	}
	evidence, err := s.repository.FindEvidenceV2ByID(ctx, runID)
	if err != nil {
		return nil, err
	}
	expectedVersion := evidence.Version()
	if err := mutation(evidence); err != nil {
		return nil, err
	}
	if err := s.repository.SaveEvidenceV2(ctx, evidence, expectedVersion); err != nil {
		return nil, err
	}
	return evidence, nil
}
