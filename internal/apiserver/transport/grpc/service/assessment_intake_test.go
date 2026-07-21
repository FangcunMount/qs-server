package service

import (
	"context"
	"errors"
	"testing"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/evaluation"
	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	evaluationintake "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/intake"
	journey "github.com/FangcunMount/qs-server/internal/apiserver/application/journey/assessmentintake"
	appanswersheet "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/answersheet"
	domainanswersheet "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type resolveIntakeStub struct {
	assessment *evaluationintake.Assessment
	err        error
}

func (s resolveIntakeStub) CreateForAnswerSheet(context.Context, evaluationintake.CreateCommand) (*evaluationintake.Assessment, error) {
	return nil, errors.New("unexpected CreateForAnswerSheet")
}
func (s resolveIntakeStub) SubmitForEvaluation(context.Context, uint64) (*evaluationintake.Assessment, error) {
	return nil, errors.New("unexpected SubmitForEvaluation")
}
func (s resolveIntakeStub) FindByAnswerSheetID(context.Context, uint64) (*evaluationintake.Assessment, error) {
	return s.assessment, s.err
}

type resolveJourneyStub struct{}

func (resolveJourneyStub) Ensure(context.Context, journey.Command) (*journey.Result, error) {
	return nil, errors.New("unexpected Ensure")
}

type resolveSheetStub struct {
	sheet *appanswersheet.AnswerSheetResult
	err   error
}

func (s resolveSheetStub) GetByID(context.Context, uint64) (*appanswersheet.AnswerSheetResult, error) {
	return s.sheet, s.err
}
func (s resolveSheetStub) List(context.Context, appanswersheet.ListAnswerSheetsDTO) (*appanswersheet.AnswerSheetSummaryListResult, error) {
	return nil, nil
}
func (s resolveSheetStub) Delete(context.Context, uint64) error { return nil }

func TestResolveAssessmentByAnswerSheetIDReadinessPhases(t *testing.T) {
	t.Parallel()

	reason := "intake failed"
	cases := []struct {
		name             string
		intake           resolveIntakeStub
		sheets           resolveSheetStub
		wantPhase        string
		wantAssessmentID uint64
		wantStatus       string
		wantFailure      string
		wantCode         codes.Code
	}{
		{
			name: "submitted ready",
			intake: resolveIntakeStub{assessment: &evaluationintake.Assessment{
				ID: 99, TesteeID: 7, Status: assessmentStatusSubmitted,
			}},
			wantPhase: readinessPhaseReady, wantAssessmentID: 99, wantStatus: assessmentStatusSubmitted,
		},
		{
			name: "evaluated ready",
			intake: resolveIntakeStub{assessment: &evaluationintake.Assessment{
				ID: 99, TesteeID: 7, Status: assessmentStatusEvaluated,
			}},
			wantPhase: readinessPhaseReady, wantAssessmentID: 99, wantStatus: assessmentStatusEvaluated,
		},
		{
			name: "pending assessment",
			intake: resolveIntakeStub{assessment: &evaluationintake.Assessment{
				ID: 99, TesteeID: 7, Status: assessmentStatusPending,
			}},
			wantPhase: readinessPhasePending, wantAssessmentID: 99, wantStatus: assessmentStatusPending,
		},
		{
			name: "failed assessment",
			intake: resolveIntakeStub{assessment: &evaluationintake.Assessment{
				ID: 99, TesteeID: 7, Status: assessmentStatusFailed, FailureReason: &reason,
			}},
			wantPhase: readinessPhaseFailed, wantAssessmentID: 99, wantStatus: assessmentStatusFailed, wantFailure: reason,
		},
		{
			name:   "independent admission no assessment required",
			intake: resolveIntakeStub{err: evalerrors.AssessmentNotFound(errors.New("missing"), "assessment missing")},
			sheets: resolveSheetStub{sheet: &appanswersheet.AnswerSheetResult{
				ID: 42, TesteeID: 7, AdmissionPurpose: string(domainanswersheet.AdmissionPurposeIndependentQuestionnaire),
			}},
			wantPhase: readinessPhaseNoAssessmentRequired, wantAssessmentID: 0,
		},
		{
			name:   "assessment admission pending ensure",
			intake: resolveIntakeStub{err: evalerrors.AssessmentNotFound(errors.New("missing"), "assessment missing")},
			sheets: resolveSheetStub{sheet: &appanswersheet.AnswerSheetResult{
				ID: 42, TesteeID: 7, AdmissionPurpose: string(domainanswersheet.AdmissionPurposeAssessment),
			}},
			wantPhase: readinessPhasePending, wantAssessmentID: 0,
		},
		{
			name:   "legacy sheet without admission pending",
			intake: resolveIntakeStub{err: evalerrors.AssessmentNotFound(errors.New("missing"), "assessment missing")},
			sheets: resolveSheetStub{sheet: &appanswersheet.AnswerSheetResult{
				ID: 42, TesteeID: 7,
			}},
			wantPhase: readinessPhasePending, wantAssessmentID: 0,
		},
		{
			name:     "dependency error surfaces",
			intake:   resolveIntakeStub{err: evalerrors.Database(errors.New("timeout"), "db timeout")},
			wantCode: codes.Internal,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			svc := NewAssessmentIntakeService(resolveJourneyStub{}, tc.intake, tc.sheets)
			got, err := svc.ResolveAssessmentByAnswerSheetID(context.Background(), &pb.ResolveAssessmentByAnswerSheetIDRequest{AnswerSheetId: 42})
			if tc.wantCode != codes.OK {
				if status.Code(err) != tc.wantCode {
					t.Fatalf("error = %v, want code %v", err, tc.wantCode)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveAssessmentByAnswerSheetID() error = %v", err)
			}
			if got.GetReadinessPhase() != tc.wantPhase || got.GetAssessmentId() != tc.wantAssessmentID {
				t.Fatalf("got phase=%q id=%d, want phase=%q id=%d", got.GetReadinessPhase(), got.GetAssessmentId(), tc.wantPhase, tc.wantAssessmentID)
			}
			if tc.wantStatus != "" && got.GetAssessmentStatus() != tc.wantStatus {
				t.Fatalf("assessment_status = %q, want %q", got.GetAssessmentStatus(), tc.wantStatus)
			}
			if got.GetFailureReason() != tc.wantFailure {
				t.Fatalf("failure_reason = %q, want %q", got.GetFailureReason(), tc.wantFailure)
			}
			if got.GetTesteeId() != 7 {
				t.Fatalf("testee_id = %d, want 7", got.GetTesteeId())
			}
		})
	}
}
