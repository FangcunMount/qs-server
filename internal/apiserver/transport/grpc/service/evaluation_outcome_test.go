package service

import (
	"context"
	"testing"

	pkgerrors "github.com/FangcunMount/component-base/pkg/errors"
	pb "github.com/FangcunMount/qs-server/api/grpc/gen/evaluation"
	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	assessmentApp "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/assessment"
	interpretationParticipant "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/participant"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestEvaluationServiceGetAssessmentReportWithTesteeRejectsWrongTestee(t *testing.T) {
	svc := &EvaluationService{
		participantReports: &fakeParticipantReportService{err: evalerrors.Forbidden("无权访问此测评")},
	}

	_, err := svc.GetAssessmentReport(context.Background(), &pb.GetAssessmentReportRequest{
		TesteeId:     7,
		AssessmentId: 42,
	})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("code = %v, want PermissionDenied", status.Code(err))
	}
}

func TestEvaluationServiceGetAssessmentReportWithTesteeReturnsReportForOwner(t *testing.T) {
	reportSvc := &fakeParticipantReportService{
		report: &interpretationParticipant.Report{AssessmentID: 42},
	}
	svc := &EvaluationService{
		participantReports: reportSvc,
	}

	resp, err := svc.GetAssessmentReport(context.Background(), &pb.GetAssessmentReportRequest{
		TesteeId:     7,
		AssessmentId: 42,
	})
	if err != nil {
		t.Fatalf("GetAssessmentReport() error = %v", err)
	}
	if resp.GetReport().GetAssessmentId() != 42 {
		t.Fatalf("assessment_id = %d, want 42", resp.GetReport().GetAssessmentId())
	}
	if reportSvc.calls != 1 || reportSvc.assessmentID != 42 {
		t.Fatalf("unexpected report query call: %#v", reportSvc)
	}
}

func TestEvaluationServiceGetAssessmentReportRequiresTestee(t *testing.T) {
	svc := &EvaluationService{participantReports: &fakeParticipantReportService{}}
	_, err := svc.GetAssessmentReport(context.Background(), &pb.GetAssessmentReportRequest{AssessmentId: 99})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument", status.Code(err))
	}
}

type fakeTesteeAssessmentQueryService struct {
	getMyAssessment func(ctx context.Context, testeeID, assessmentID uint64) (*assessmentApp.AssessmentResult, error)
}

func (s *fakeTesteeAssessmentQueryService) GetMine(ctx context.Context, testeeID, assessmentID uint64) (*assessmentApp.AssessmentResult, error) {
	if s.getMyAssessment == nil {
		return nil, pkgerrors.WithCode(errorCode.ErrAssessmentNotFound, "assessment not found")
	}
	return s.getMyAssessment(ctx, testeeID, assessmentID)
}

func (s *fakeTesteeAssessmentQueryService) ListMine(context.Context, assessmentApp.ListMyAssessmentsDTO) (*assessmentApp.AssessmentListResult, error) {
	panic("unexpected ListMine call")
}

type fakeParticipantReportService struct {
	calls        int
	assessmentID uint64
	report       *interpretationParticipant.Report
	err          error
}

func (s *fakeParticipantReportService) GetMyReport(_ context.Context, actor interpretationParticipant.Actor, query interpretationParticipant.GetQuery) (*interpretationParticipant.Report, error) {
	s.calls++
	s.assessmentID = query.AssessmentID
	if s.err != nil {
		return nil, s.err
	}
	return s.report, nil
}

func (s *fakeParticipantReportService) ListMyReports(context.Context, interpretationParticipant.Actor, interpretationParticipant.ListQuery) (*interpretationParticipant.ListResult, error) {
	panic("unexpected ListOutcomeByTesteeID call")
}
