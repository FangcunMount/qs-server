package service

import (
	"context"
	"testing"

	pkgerrors "github.com/FangcunMount/component-base/pkg/errors"
	pb "github.com/FangcunMount/qs-server/api/grpc/gen/evaluation"
	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	assessmentApp "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/assessment"
	interpretationApp "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestEvaluationServiceGetAssessmentReportWithTesteeRejectsWrongTestee(t *testing.T) {
	svc := &EvaluationService{
		testeeQueryService: &fakeTesteeAssessmentQueryService{
			getMyAssessment: func(context.Context, uint64, uint64) (*assessmentApp.AssessmentResult, error) {
				return nil, evalerrors.Forbidden("无权访问此测评")
			},
		},
		reportQueryService: &fakeReportQueryService{},
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
	reportSvc := &fakeReportQueryService{
		report: &interpretationApp.ReportOutcomeResult{AssessmentID: 42},
	}
	svc := &EvaluationService{
		testeeQueryService: &fakeTesteeAssessmentQueryService{
			getMyAssessment: func(_ context.Context, testeeID, assessmentID uint64) (*assessmentApp.AssessmentResult, error) {
				if testeeID != 7 || assessmentID != 42 {
					t.Fatalf("unexpected ids: testee=%d assessment=%d", testeeID, assessmentID)
				}
				return &assessmentApp.AssessmentResult{ID: 42, TesteeID: 7}, nil
			},
		},
		reportQueryService: reportSvc,
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

func TestEvaluationServiceGetAssessmentReportWithoutTesteeUsesLegacyPath(t *testing.T) {
	reportSvc := &fakeReportQueryService{
		legacyReport: &interpretationApp.ReportResult{AssessmentID: 99},
	}
	svc := &EvaluationService{
		reportQueryService: reportSvc,
	}

	resp, err := svc.GetAssessmentReport(context.Background(), &pb.GetAssessmentReportRequest{AssessmentId: 99})
	if err != nil {
		t.Fatalf("GetAssessmentReport() error = %v", err)
	}
	if resp.GetReport().GetAssessmentId() != 99 {
		t.Fatalf("assessment_id = %d, want 99", resp.GetReport().GetAssessmentId())
	}
	if reportSvc.legacyCalls != 1 {
		t.Fatalf("legacyCalls = %d, want 1", reportSvc.legacyCalls)
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

type fakeReportQueryService struct {
	calls        int
	legacyCalls  int
	assessmentID uint64
	report       *interpretationApp.ReportOutcomeResult
	legacyReport *interpretationApp.ReportResult
	err          error
}

func (s *fakeReportQueryService) GetOutcomeByAssessmentID(_ context.Context, assessmentID uint64) (*interpretationApp.ReportOutcomeResult, error) {
	s.calls++
	s.assessmentID = assessmentID
	if s.err != nil {
		return nil, s.err
	}
	return s.report, nil
}

func (s *fakeReportQueryService) GetByAssessmentID(context.Context, uint64) (*interpretationApp.ReportResult, error) {
	s.legacyCalls++
	return s.legacyReport, nil
}

func (s *fakeReportQueryService) ListByTesteeID(context.Context, interpretationApp.ListReportsDTO) (*interpretationApp.ReportListResult, error) {
	panic("unexpected ListByTesteeID call")
}

func (s *fakeReportQueryService) ListOutcomeByTesteeID(context.Context, interpretationApp.ListReportsDTO) (*interpretationApp.ReportOutcomeListResult, error) {
	panic("unexpected ListOutcomeByTesteeID call")
}
