package service

import (
	"context"
	"testing"

	pkgerrors "github.com/FangcunMount/component-base/pkg/errors"
	assessmentApp "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/assessment"
	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	pb "github.com/FangcunMount/qs-server/internal/apiserver/interface/grpc/proto/evaluation"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestEvaluationServiceGetAssessmentReportV2RequiresTesteeID(t *testing.T) {
	svc := &EvaluationService{
		submissionService:  &fakeAssessmentSubmissionService{},
		reportQueryService: &fakeReportQueryService{},
	}

	_, err := svc.GetAssessmentReportV2(context.Background(), &pb.GetAssessmentReportV2Request{AssessmentId: 1})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument", status.Code(err))
	}
}

func TestEvaluationServiceGetAssessmentReportV2RejectsWrongTestee(t *testing.T) {
	svc := &EvaluationService{
		submissionService: &fakeAssessmentSubmissionService{
			getMyAssessment: func(context.Context, uint64, uint64) (*assessmentApp.AssessmentResult, error) {
				return nil, evalerrors.Forbidden("无权访问此测评")
			},
		},
		reportQueryService: &fakeReportQueryService{},
	}

	_, err := svc.GetAssessmentReportV2(context.Background(), &pb.GetAssessmentReportV2Request{
		TesteeId:     7,
		AssessmentId: 42,
	})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("code = %v, want PermissionDenied", status.Code(err))
	}
}

func TestEvaluationServiceGetAssessmentReportV2ReturnsReportForOwner(t *testing.T) {
	reportSvc := &fakeReportQueryService{
		report: &assessmentApp.ReportV2Result{AssessmentID: 42},
	}
	svc := &EvaluationService{
		submissionService: &fakeAssessmentSubmissionService{
			getMyAssessment: func(_ context.Context, testeeID, assessmentID uint64) (*assessmentApp.AssessmentResult, error) {
				if testeeID != 7 || assessmentID != 42 {
					t.Fatalf("unexpected ids: testee=%d assessment=%d", testeeID, assessmentID)
				}
				return &assessmentApp.AssessmentResult{ID: 42, TesteeID: 7}, nil
			},
		},
		reportQueryService: reportSvc,
	}

	resp, err := svc.GetAssessmentReportV2(context.Background(), &pb.GetAssessmentReportV2Request{
		TesteeId:     7,
		AssessmentId: 42,
	})
	if err != nil {
		t.Fatalf("GetAssessmentReportV2() error = %v", err)
	}
	if resp.GetReport().GetAssessmentId() != 42 {
		t.Fatalf("assessment_id = %d, want 42", resp.GetReport().GetAssessmentId())
	}
	if reportSvc.calls != 1 || reportSvc.assessmentID != 42 {
		t.Fatalf("unexpected report query call: %#v", reportSvc)
	}
}

type fakeAssessmentSubmissionService struct {
	getMyAssessment func(ctx context.Context, testeeID, assessmentID uint64) (*assessmentApp.AssessmentResult, error)
}

func (s *fakeAssessmentSubmissionService) Create(context.Context, assessmentApp.CreateAssessmentDTO) (*assessmentApp.AssessmentResult, error) {
	panic("unexpected Create call")
}

func (s *fakeAssessmentSubmissionService) Submit(context.Context, uint64) (*assessmentApp.AssessmentResult, error) {
	panic("unexpected Submit call")
}

func (s *fakeAssessmentSubmissionService) GetMyAssessment(ctx context.Context, testeeID, assessmentID uint64) (*assessmentApp.AssessmentResult, error) {
	if s.getMyAssessment == nil {
		return nil, pkgerrors.WithCode(errorCode.ErrAssessmentNotFound, "assessment not found")
	}
	return s.getMyAssessment(ctx, testeeID, assessmentID)
}

func (s *fakeAssessmentSubmissionService) GetMyAssessmentByAnswerSheetID(context.Context, uint64) (*assessmentApp.AssessmentResult, error) {
	panic("unexpected GetMyAssessmentByAnswerSheetID call")
}

func (s *fakeAssessmentSubmissionService) ListMyAssessments(context.Context, assessmentApp.ListMyAssessmentsDTO) (*assessmentApp.AssessmentListResult, error) {
	panic("unexpected ListMyAssessments call")
}

type fakeReportQueryService struct {
	calls        int
	assessmentID uint64
	report       *assessmentApp.ReportV2Result
	err          error
}

func (s *fakeReportQueryService) GetV2ByAssessmentID(_ context.Context, assessmentID uint64) (*assessmentApp.ReportV2Result, error) {
	s.calls++
	s.assessmentID = assessmentID
	if s.err != nil {
		return nil, s.err
	}
	return s.report, nil
}

func (s *fakeReportQueryService) GetByAssessmentID(context.Context, uint64) (*assessmentApp.ReportResult, error) {
	panic("unexpected GetByAssessmentID call")
}

func (s *fakeReportQueryService) ListByTesteeID(context.Context, assessmentApp.ListReportsDTO) (*assessmentApp.ReportListResult, error) {
	panic("unexpected ListByTesteeID call")
}

func (s *fakeReportQueryService) ListV2ByTesteeID(context.Context, assessmentApp.ListReportsDTO) (*assessmentApp.ReportV2ListResult, error) {
	panic("unexpected ListV2ByTesteeID call")
}
