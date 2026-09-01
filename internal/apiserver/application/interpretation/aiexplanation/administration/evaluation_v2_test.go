package administration

import (
	"context"
	"testing"
	"time"

	cberrors "github.com/FangcunMount/component-base/pkg/errors"
	appevaluation "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/evaluation"
	domainevaluation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/evaluation"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestStartEvaluationV2FreezesCurrentPoliciesAndTrustedAudit(t *testing.T) {
	starter := &evaluationV2StarterStub{}
	evidence := &evaluationV2EvidenceStub{}
	committer := &evaluationV2ResolutionCommitterStub{}
	service := NewService(&reviewWorkflowStub{}, &governanceStub{}, &accessStub{},
		WithEvaluationV2(starter, evidence, committer, func() meta.ID { return meta.ID(701) }),
	)

	result, err := service.StartEvaluationV2(context.Background(), Actor{OrgID: 7, OperatorUserID: 42}, StartEvaluationV2Command{
		Confirm: true, ExpectedProviderInvocations: 140, Reason: "evaluate the frozen v2 release",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || starter.command.RunID != meta.ID(701) || starter.command.OrgID != 7 || starter.command.RequestedBy != "user:42" {
		t.Fatalf("start result/command = %#v / %#v", result, starter.command)
	}
	if starter.command.ExecutionPolicy.WorstCaseProviderCalls() != 140 || starter.command.ExecutionPolicy.Validate() != nil || starter.command.GatePolicy.Validate() != nil {
		t.Fatalf("invalid frozen policies = %#v / %#v", starter.command.ExecutionPolicy, starter.command.GatePolicy)
	}

	_, err = service.StartEvaluationV2(context.Background(), Actor{OrgID: 7, OperatorUserID: 42}, StartEvaluationV2Command{
		Confirm: true, ExpectedProviderInvocations: 70, Reason: "stale v1 confirmation",
	})
	if coder := cberrors.ParseCoder(err); coder == nil || coder.Code() != code.ErrInvalidArgument {
		t.Fatalf("stale v1 confirmation error = %v", err)
	}
}

func TestEvaluationV2ReviewUsesCandidateIdentityAndDistinctTrustedReviewers(t *testing.T) {
	now := time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
	evidence := &evaluationV2EvidenceStub{value: reviewableEvaluationV2()}
	svc := NewService(&reviewWorkflowStub{}, &governanceStub{}, &accessStub{},
		WithEvaluationV2(&evaluationV2StarterStub{}, evidence, &evaluationV2ResolutionCommitterStub{}, nil),
	)
	svc.(*service).now = func() time.Time { return now }

	result, err := svc.RecordReviewV2(context.Background(), Actor{OrgID: 7, OperatorUserID: 42}, meta.ID(701), ReviewV2Command{
		CandidateID: "candidate:case-1:slot-1", Role: domainevaluation.ReviewRoleAssessmentSemantics,
		Decision: domainevaluation.ReviewDecisionApprove, Reason: "facts match",
	})
	if err != nil || result == nil || evidence.recorded.Reviewer != "user:42" || !evidence.recorded.ReviewedAt.Equal(now) {
		t.Fatalf("review result = %#v, recorded = %#v, error = %v", result, evidence.recorded, err)
	}

	evidence.value.HumanReviews = []domainevaluation.CandidateHumanReview{{
		CandidateID: "candidate:case-1:slot-1", Role: domainevaluation.ReviewRoleAssessmentSemantics,
		Reviewer: "user:42", Decision: domainevaluation.ReviewDecisionApprove, ReviewedAt: now, Reason: "facts match",
	}}
	_, err = svc.RecordReviewV2(context.Background(), Actor{OrgID: 7, OperatorUserID: 42}, meta.ID(701), ReviewV2Command{
		CandidateID: "candidate:case-1:slot-1", Role: domainevaluation.ReviewRoleSafetyProduct,
		Decision: domainevaluation.ReviewDecisionApprove, Reason: "safe",
	})
	if coder := cberrors.ParseCoder(err); coder == nil || coder.Code() != code.ErrConflict {
		t.Fatalf("same reviewer error = %v", err)
	}
}

func TestEvaluationV2BatchReviewValidatesAllCandidatesAndCommitsOnce(t *testing.T) {
	now := time.Date(2026, 9, 1, 8, 30, 0, 0, time.UTC)
	current := reviewableEvaluationV2()
	current.Slots = append(current.Slots, domainevaluation.CandidateSlot{
		CaseID: "case-1", Ordinal: 2,
		Candidate: &domainevaluation.Candidate{ID: "candidate:case-1:slot-2", ReviewReady: true},
	})
	evidence := &evaluationV2EvidenceStub{value: current}
	svc := NewService(&reviewWorkflowStub{}, &governanceStub{}, &accessStub{},
		WithEvaluationV2(&evaluationV2StarterStub{}, evidence, &evaluationV2ResolutionCommitterStub{}, nil),
	)
	svc.(*service).now = func() time.Time { return now }

	result, err := svc.RecordReviewsV2(context.Background(), Actor{OrgID: 7, OperatorUserID: 42}, meta.ID(701), ReviewV2BatchCommand{
		Role: domainevaluation.ReviewRoleAssessmentSemantics,
		Reviews: []ReviewV2BatchItemCommand{
			{CandidateID: "candidate:case-1:slot-1", Decision: domainevaluation.ReviewDecisionApprove, Reason: " facts match "},
			{CandidateID: "candidate:case-1:slot-2", Decision: domainevaluation.ReviewDecisionReject, Reason: "unsupported inference"},
		},
	})
	if err != nil || result == nil || evidence.recordBatchCalls != 1 || len(evidence.recordedBatch) != 2 {
		t.Fatalf("batch result=%#v calls=%d reviews=%#v error=%v", result, evidence.recordBatchCalls, evidence.recordedBatch, err)
	}
	for _, review := range evidence.recordedBatch {
		if review.Role != domainevaluation.ReviewRoleAssessmentSemantics || review.Reviewer != "user:42" || !review.ReviewedAt.Equal(now) {
			t.Fatalf("trusted batch audit is invalid: %#v", review)
		}
	}
	if evidence.recordedBatch[0].Reason != "facts match" {
		t.Fatalf("trimmed reason = %q", evidence.recordedBatch[0].Reason)
	}

	evidence.recordedBatch = nil
	evidence.recordBatchCalls = 0
	_, err = svc.RecordReviewsV2(context.Background(), Actor{OrgID: 7, OperatorUserID: 42}, meta.ID(701), ReviewV2BatchCommand{
		Role: domainevaluation.ReviewRoleAssessmentSemantics,
		Reviews: []ReviewV2BatchItemCommand{
			{CandidateID: "candidate:case-1:slot-1", Decision: domainevaluation.ReviewDecisionApprove, Reason: "facts match"},
			{CandidateID: "candidate:missing", Decision: domainevaluation.ReviewDecisionApprove, Reason: "missing"},
		},
	})
	if coder := cberrors.ParseCoder(err); coder == nil || coder.Code() != code.ErrConflict || evidence.recordBatchCalls != 0 {
		t.Fatalf("invalid target error=%v calls=%d", err, evidence.recordBatchCalls)
	}
}

func TestEvaluationV2BatchReviewRejectsDuplicateCandidateBeforeWrite(t *testing.T) {
	evidence := &evaluationV2EvidenceStub{value: reviewableEvaluationV2()}
	svc := NewService(&reviewWorkflowStub{}, &governanceStub{}, &accessStub{},
		WithEvaluationV2(&evaluationV2StarterStub{}, evidence, &evaluationV2ResolutionCommitterStub{}, nil),
	)

	_, err := svc.RecordReviewsV2(context.Background(), Actor{OrgID: 7, OperatorUserID: 42}, meta.ID(701), ReviewV2BatchCommand{
		Role: domainevaluation.ReviewRoleAssessmentSemantics,
		Reviews: []ReviewV2BatchItemCommand{
			{CandidateID: "candidate:case-1:slot-1", Decision: domainevaluation.ReviewDecisionApprove, Reason: "facts match"},
			{CandidateID: "candidate:case-1:slot-1", Decision: domainevaluation.ReviewDecisionApprove, Reason: "duplicate"},
		},
	})
	if coder := cberrors.ParseCoder(err); coder == nil || coder.Code() != code.ErrInvalidArgument || evidence.recordBatchCalls != 0 {
		t.Fatalf("duplicate error=%v calls=%d", err, evidence.recordBatchCalls)
	}
}

func TestResolveResultUnknownV2RequiresExplicitDuplicateCallRiskAndUsesDurableCommitter(t *testing.T) {
	now := time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
	evidence := &evaluationV2EvidenceStub{value: &domainevaluation.PromptEvaluationEvidenceV2{
		RunID: meta.ID(701), Status: domainevaluation.EvidenceStatusBlocked, UnresolvedResultUnknownCount: 1,
		Audit: domainevaluation.EvidenceRunAudit{OrganizationID: 7},
	}}
	committer := &evaluationV2ResolutionCommitterStub{value: evidence.value}
	svc := NewService(&reviewWorkflowStub{}, &governanceStub{}, &accessStub{},
		WithEvaluationV2(&evaluationV2StarterStub{}, evidence, committer, nil),
	)
	svc.(*service).now = func() time.Time { return now }

	_, err := svc.ResolveResultUnknownV2(context.Background(), Actor{OrgID: 7, OperatorUserID: 42}, meta.ID(701), ResolveResultUnknownV2Command{
		ExecutionID: "generation:case-1:slot-1:1", Decision: domainevaluation.ResultUnknownAuthorizeReplacement,
		Confirm: true, Reason: "resume after manual inspection",
	})
	if coder := cberrors.ParseCoder(err); coder == nil || coder.Code() != code.ErrInvalidArgument {
		t.Fatalf("missing risk acknowledgement error = %v", err)
	}

	_, err = svc.ResolveResultUnknownV2(context.Background(), Actor{OrgID: 7, OperatorUserID: 42}, meta.ID(701), ResolveResultUnknownV2Command{
		ExecutionID: "generation:case-1:slot-1:1", Decision: domainevaluation.ResultUnknownAuthorizeReplacement,
		Confirm: true, AcknowledgedDuplicateCallAndCostRisk: true, Reason: "resume after manual inspection",
	})
	if err != nil || committer.resolution.Actor != "user:42" || !committer.resolution.ResolvedAt.Equal(now) {
		t.Fatalf("resolution = %#v, error = %v", committer.resolution, err)
	}
}

type evaluationV2StarterStub struct {
	command appevaluation.OnlineStartV2Command
}

func (s *evaluationV2StarterStub) StartRequestedV2(_ context.Context, command appevaluation.OnlineStartV2Command) (*appevaluation.OnlineRunV2Result, error) {
	s.command = command
	return &appevaluation.OnlineRunV2Result{Evidence: &domainevaluation.PromptEvaluationEvidenceV2{
		RunID: command.RunID, Status: domainevaluation.EvidenceStatusCollecting,
		Audit: domainevaluation.EvidenceRunAudit{OrganizationID: command.OrgID, RequestedBy: command.RequestedBy, RequestReason: command.Reason},
	}}, nil
}

type evaluationV2EvidenceStub struct {
	value            *domainevaluation.PromptEvaluationEvidenceV2
	recorded         domainevaluation.CandidateHumanReview
	recordedBatch    []domainevaluation.CandidateHumanReview
	recordBatchCalls int
	finalized        bool
}

func (s *evaluationV2EvidenceStub) Find(context.Context, meta.ID) (*domainevaluation.PromptEvaluationEvidenceV2, error) {
	return s.value, nil
}

func (s *evaluationV2EvidenceStub) RecordHumanReview(_ context.Context, _ meta.ID, value domainevaluation.CandidateHumanReview) (*domainevaluation.PromptEvaluationEvidenceV2, error) {
	s.recorded = value
	return s.value, nil
}

func (s *evaluationV2EvidenceStub) RecordHumanReviews(_ context.Context, _ meta.ID, values []domainevaluation.CandidateHumanReview) (*domainevaluation.PromptEvaluationEvidenceV2, error) {
	s.recordBatchCalls++
	s.recordedBatch = append([]domainevaluation.CandidateHumanReview(nil), values...)
	if len(values) == 1 {
		s.recorded = values[0]
	}
	return s.value, nil
}

func (s *evaluationV2EvidenceStub) Finalize(context.Context, meta.ID, string, string, time.Time) (*domainevaluation.PromptEvaluationEvidenceV2, error) {
	s.finalized = true
	return s.value, nil
}

type evaluationV2ResolutionCommitterStub struct {
	value      *domainevaluation.PromptEvaluationEvidenceV2
	resolution domainevaluation.ResultUnknownResolution
}

func (s *evaluationV2ResolutionCommitterStub) CommitResultUnknownResolutionV2(_ context.Context, _ meta.ID, value domainevaluation.ResultUnknownResolution) (*domainevaluation.PromptEvaluationEvidenceV2, error) {
	s.resolution = value
	return s.value, nil
}

func reviewableEvaluationV2() *domainevaluation.PromptEvaluationEvidenceV2 {
	return &domainevaluation.PromptEvaluationEvidenceV2{
		RunID: meta.ID(701), Status: domainevaluation.EvidenceStatusAwaitingReview,
		Audit: domainevaluation.EvidenceRunAudit{OrganizationID: 7},
		Slots: []domainevaluation.CandidateSlot{{
			CaseID: "case-1", Ordinal: 1,
			Candidate: &domainevaluation.Candidate{ID: "candidate:case-1:slot-1", ReviewReady: true},
		}},
	}
}
