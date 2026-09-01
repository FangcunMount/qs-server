package administration

import (
	"context"
	"strings"
	"time"

	cberrors "github.com/FangcunMount/component-base/pkg/errors"
	appevaluation "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/evaluation"
	domainevaluation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/evaluation"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type EvaluationV2Starter interface {
	StartRequestedV2(context.Context, appevaluation.OnlineStartV2Command) (*appevaluation.OnlineRunV2Result, error)
}

type EvaluationV2EvidenceWorkflow interface {
	Find(context.Context, meta.ID) (*domainevaluation.PromptEvaluationEvidenceV2, error)
	RecordHumanReview(context.Context, meta.ID, domainevaluation.CandidateHumanReview) (*domainevaluation.PromptEvaluationEvidenceV2, error)
	RecordHumanReviews(context.Context, meta.ID, []domainevaluation.CandidateHumanReview) (*domainevaluation.PromptEvaluationEvidenceV2, error)
	Finalize(context.Context, meta.ID, string, string, time.Time) (*domainevaluation.PromptEvaluationEvidenceV2, error)
}

const maxReviewV2BatchSize = 35

type EvaluationV2ResolutionCommitter interface {
	CommitResultUnknownResolutionV2(context.Context, meta.ID, domainevaluation.ResultUnknownResolution) (*domainevaluation.PromptEvaluationEvidenceV2, error)
}

func (s *service) StartEvaluationV2(ctx context.Context, actor Actor, command StartEvaluationV2Command) (*domainevaluation.PromptEvaluationEvidenceV2, error) {
	policy := domainevaluation.CurrentEvaluationExecutionPolicy()
	command.Reason = strings.TrimSpace(command.Reason)
	if actor.OrgID <= 0 || actor.OperatorUserID <= 0 || !command.Confirm ||
		command.ExpectedProviderInvocations != policy.WorstCaseProviderCalls() ||
		command.Reason == "" || len(command.Reason) > maxAuditReasonLength {
		return nil, cberrors.WithCode(code.ErrInvalidArgument, "AI explanation evaluation v2 cost confirmation is invalid")
	}
	if s == nil || s.starterV2 == nil || s.evidenceV2 == nil || s.resolutionCommitterV2 == nil || s.access == nil || s.newID == nil {
		return nil, cberrors.WithCode(code.ErrUnsupportedOperation, "AI explanation online evaluation v2 is disabled")
	}
	if err := s.access.AuthorizeGovernance(ctx, actor); err != nil {
		return nil, err
	}
	result, err := s.starterV2.StartRequestedV2(ctx, appevaluation.OnlineStartV2Command{
		RunID: s.newID(), OrgID: actor.OrgID, RequestedBy: actor.Subject(), Reason: command.Reason,
		ExecutionPolicy: policy, GatePolicy: domainevaluation.CurrentReleaseGatePolicy(),
	})
	if err != nil {
		return nil, mapKnownError(err)
	}
	if result == nil || result.Evidence == nil {
		return nil, cberrors.WithCode(code.ErrUnknown, "AI explanation evaluation v2 start returned no evidence")
	}
	if err := validateEvaluationV2Org(result.Evidence, actor); err != nil {
		return nil, err
	}
	return result.Evidence, nil
}

func (s *service) FindEvaluationV2(ctx context.Context, actor Actor, runID meta.ID) (*domainevaluation.PromptEvaluationEvidenceV2, error) {
	if actor.OrgID <= 0 || actor.OperatorUserID <= 0 || runID.IsZero() {
		return nil, cberrors.WithCode(code.ErrInvalidArgument, "AI explanation evaluation v2 identity is invalid")
	}
	if s == nil || s.evidenceV2 == nil || s.access == nil {
		return nil, cberrors.WithCode(code.ErrUnsupportedOperation, "AI explanation evaluation v2 is disabled")
	}
	if err := s.access.AuthorizeRead(ctx, actor); err != nil {
		return nil, err
	}
	value, err := s.evidenceV2.Find(ctx, runID)
	if err != nil {
		return nil, mapKnownError(err)
	}
	if err := validateEvaluationV2Org(value, actor); err != nil {
		return nil, err
	}
	return value, nil
}

func (s *service) RecordReviewV2(ctx context.Context, actor Actor, runID meta.ID, command ReviewV2Command) (*domainevaluation.PromptEvaluationEvidenceV2, error) {
	return s.recordReviewsV2(ctx, actor, runID, command.Role, []ReviewV2BatchItemCommand{{
		CandidateID: command.CandidateID, Decision: command.Decision, Reason: command.Reason,
	}}, false)
}

func (s *service) RecordReviewsV2(ctx context.Context, actor Actor, runID meta.ID, command ReviewV2BatchCommand) (*domainevaluation.PromptEvaluationEvidenceV2, error) {
	return s.recordReviewsV2(ctx, actor, runID, command.Role, command.Reviews, true)
}

func (s *service) recordReviewsV2(ctx context.Context, actor Actor, runID meta.ID, role domainevaluation.ReviewRole, commands []ReviewV2BatchItemCommand, batch bool) (*domainevaluation.PromptEvaluationEvidenceV2, error) {
	if actor.OrgID <= 0 || actor.OperatorUserID <= 0 || runID.IsZero() || !validRole(role) || len(commands) == 0 || len(commands) > maxReviewV2BatchSize {
		return nil, invalidReviewV2Command(batch)
	}
	seen := make(map[string]struct{}, len(commands))
	for index := range commands {
		commands[index].CandidateID = strings.TrimSpace(commands[index].CandidateID)
		commands[index].Reason = strings.TrimSpace(commands[index].Reason)
		if commands[index].CandidateID == "" || !validDecision(commands[index].Decision) || commands[index].Reason == "" || len(commands[index].Reason) > maxAuditReasonLength {
			return nil, invalidReviewV2Command(batch)
		}
		if _, duplicated := seen[commands[index].CandidateID]; duplicated {
			return nil, cberrors.WithCode(code.ErrInvalidArgument, "AI explanation evaluation v2 review batch contains duplicate candidates")
		}
		seen[commands[index].CandidateID] = struct{}{}
	}
	if s == nil || s.evidenceV2 == nil || s.access == nil || s.now == nil {
		return nil, cberrors.WithCode(code.ErrUnsupportedOperation, "AI explanation evaluation v2 review is disabled")
	}
	if err := s.access.AuthorizeReview(ctx, actor, role); err != nil {
		return nil, err
	}
	current, err := s.evidenceV2.Find(ctx, runID)
	if err != nil {
		return nil, mapKnownError(err)
	}
	if err := validateEvaluationV2Org(current, actor); err != nil {
		return nil, err
	}
	if current.Status != domainevaluation.EvidenceStatusAwaitingReview {
		return nil, cberrors.WithCode(code.ErrConflict, "AI explanation evaluation v2 candidate is not reviewable")
	}
	reviewer := actor.Subject()
	for _, command := range commands {
		if !evidenceHasReviewReadyCandidate(current, command.CandidateID) {
			return nil, cberrors.WithCode(code.ErrConflict, "AI explanation evaluation v2 candidate is not reviewable")
		}
		for _, existing := range current.HumanReviews {
			if existing.CandidateID != command.CandidateID {
				continue
			}
			if existing.Role == role {
				return nil, cberrors.WithCode(code.ErrConflict, "AI explanation evaluation v2 candidate role is already reviewed")
			}
			if existing.Reviewer == reviewer {
				return nil, cberrors.WithCode(code.ErrConflict, "AI explanation evaluation v2 requires distinct reviewers per candidate")
			}
		}
	}
	reviewedAt := s.now().UTC()
	reviews := make([]domainevaluation.CandidateHumanReview, 0, len(commands))
	for _, command := range commands {
		reviews = append(reviews, domainevaluation.CandidateHumanReview{
			CandidateID: command.CandidateID, Role: role, Reviewer: reviewer,
			Decision: command.Decision, ReviewedAt: reviewedAt, Reason: command.Reason,
		})
	}
	value, err := s.evidenceV2.RecordHumanReviews(ctx, runID, reviews)
	return value, mapKnownError(err)
}

func invalidReviewV2Command(batch bool) error {
	if batch {
		return cberrors.WithCode(code.ErrInvalidArgument, "AI explanation evaluation v2 review batch is invalid")
	}
	return cberrors.WithCode(code.ErrInvalidArgument, "AI explanation evaluation v2 review command is invalid")
}

func (s *service) FinalizeEvaluationV2(ctx context.Context, actor Actor, runID meta.ID, reason string) (*domainevaluation.PromptEvaluationEvidenceV2, error) {
	reason = strings.TrimSpace(reason)
	if actor.OrgID <= 0 || actor.OperatorUserID <= 0 || runID.IsZero() || reason == "" || len(reason) > maxAuditReasonLength {
		return nil, cberrors.WithCode(code.ErrInvalidArgument, "AI explanation evaluation v2 finalization is invalid")
	}
	if s == nil || s.evidenceV2 == nil || s.access == nil || s.now == nil {
		return nil, cberrors.WithCode(code.ErrUnsupportedOperation, "AI explanation evaluation v2 review is disabled")
	}
	if err := s.access.AuthorizeGovernance(ctx, actor); err != nil {
		return nil, err
	}
	current, err := s.evidenceV2.Find(ctx, runID)
	if err != nil {
		return nil, mapKnownError(err)
	}
	if err := validateEvaluationV2Org(current, actor); err != nil {
		return nil, err
	}
	if current.Status != domainevaluation.EvidenceStatusAwaitingReview {
		return nil, cberrors.WithCode(code.ErrConflict, "AI explanation evaluation v2 is not awaiting review")
	}
	value, err := s.evidenceV2.Finalize(ctx, runID, actor.Subject(), "human_review_finalized", s.now().UTC())
	return value, mapKnownError(err)
}

func (s *service) ResolveResultUnknownV2(ctx context.Context, actor Actor, runID meta.ID, command ResolveResultUnknownV2Command) (*domainevaluation.PromptEvaluationEvidenceV2, error) {
	command.ExecutionID, command.Reason = strings.TrimSpace(command.ExecutionID), strings.TrimSpace(command.Reason)
	if actor.OrgID <= 0 || actor.OperatorUserID <= 0 || runID.IsZero() || command.ExecutionID == "" || !command.Confirm ||
		!command.AcknowledgedDuplicateCallAndCostRisk || command.Reason == "" || len(command.Reason) > maxAuditReasonLength ||
		(command.Decision != domainevaluation.ResultUnknownAuthorizeReplacement && command.Decision != domainevaluation.ResultUnknownCancelRun) {
		return nil, cberrors.WithCode(code.ErrInvalidArgument, "AI explanation result-unknown v2 resolution is invalid")
	}
	if s == nil || s.evidenceV2 == nil || s.resolutionCommitterV2 == nil || s.access == nil || s.now == nil {
		return nil, cberrors.WithCode(code.ErrUnsupportedOperation, "AI explanation result-unknown v2 resolution is disabled")
	}
	if err := s.access.AuthorizeGovernance(ctx, actor); err != nil {
		return nil, err
	}
	current, err := s.evidenceV2.Find(ctx, runID)
	if err != nil {
		return nil, mapKnownError(err)
	}
	if err := validateEvaluationV2Org(current, actor); err != nil {
		return nil, err
	}
	if current.Status != domainevaluation.EvidenceStatusBlocked || current.UnresolvedResultUnknownCount < 1 {
		return nil, cberrors.WithCode(code.ErrConflict, "AI explanation evaluation v2 has no unresolved result_unknown")
	}
	value, err := s.resolutionCommitterV2.CommitResultUnknownResolutionV2(ctx, runID, domainevaluation.ResultUnknownResolution{
		ExecutionID: command.ExecutionID, Decision: command.Decision, Actor: actor.Subject(), Reason: command.Reason,
		AcknowledgedDuplicateCallAndCostRisk: true, ResolvedAt: s.now().UTC(),
	})
	return value, mapKnownError(err)
}

func validateEvaluationV2Org(value *domainevaluation.PromptEvaluationEvidenceV2, actor Actor) error {
	if value == nil {
		return cberrors.WithCode(code.ErrPageNotFound, "AI explanation evaluation v2 not found")
	}
	if value.Audit.OrganizationID != actor.OrgID {
		return cberrors.WithCode(code.ErrPermissionDenied, "AI explanation evaluation v2 belongs to another organization")
	}
	return nil
}

func evidenceHasReviewReadyCandidate(value *domainevaluation.PromptEvaluationEvidenceV2, candidateID string) bool {
	for _, slot := range value.Slots {
		if slot.Candidate != nil && slot.Candidate.ID == candidateID && slot.Candidate.ReviewReady {
			return true
		}
	}
	return false
}
