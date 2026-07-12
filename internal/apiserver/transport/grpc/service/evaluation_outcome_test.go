package service

import (
	"context"
	"testing"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/interpretation"
	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	interpretationParticipant "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/participant"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestParticipantReportServiceRejectsWrongTestee(t *testing.T) {
	svc := &ParticipantReportService{
		service: &fakeParticipantReportService{err: evalerrors.Forbidden("无权访问此测评")},
	}

	_, err := svc.GetAssessmentReport(context.Background(), &pb.GetAssessmentReportRequest{
		TesteeId:     7,
		AssessmentId: 42,
	})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("code = %v, want PermissionDenied", status.Code(err))
	}
}

func TestParticipantReportServiceReturnsReportForOwner(t *testing.T) {
	reportSvc := &fakeParticipantReportService{
		report: &interpretationParticipant.Report{AssessmentID: 42},
	}
	svc := &ParticipantReportService{
		service: reportSvc,
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

func TestParticipantReportServiceRequiresTestee(t *testing.T) {
	svc := &ParticipantReportService{service: &fakeParticipantReportService{}}
	_, err := svc.GetAssessmentReport(context.Background(), &pb.GetAssessmentReportRequest{AssessmentId: 99})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument", status.Code(err))
	}
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
