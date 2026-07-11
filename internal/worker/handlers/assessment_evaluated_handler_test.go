package handlers

import (
	"context"
	"io"
	"log/slog"
	"testing"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/internalapi"
)

type reportGeneratingInternalClient struct {
	fakeWorkerInternalClient
	generateReportResp *pb.GenerateReportFromAssessmentResponse
	generateReportErr  error
	outcomeID          string
}

func (f *reportGeneratingInternalClient) GenerateReportFromOutcome(
	_ context.Context,
	outcomeID string,
) (*pb.GenerateReportFromAssessmentResponse, error) {
	f.generateReportCalls++
	f.outcomeID = outcomeID
	if f.generateReportErr != nil {
		return nil, f.generateReportErr
	}
	if f.generateReportResp != nil {
		return f.generateReportResp, nil
	}
	return &pb.GenerateReportFromAssessmentResponse{Success: true, Status: "interpreted"}, nil
}

func TestHandleAssessmentEvaluated_ReportFailureWithFailedStatusAcks(t *testing.T) {
	client := &reportGeneratingInternalClient{
		generateReportResp: &pb.GenerateReportFromAssessmentResponse{
			Success: false,
			Status:  "failed",
			Message: "report generation failed",
		},
	}
	deps := &Dependencies{
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		InternalClient: client,
	}
	handler := handleAssessmentEvaluated(deps)

	err := handler(context.Background(), "assessment.evaluated", mustBuildAssessmentEvaluatedPayload(t, 2001))
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if client.generateReportCalls != 1 {
		t.Fatalf("expected 1 generate report call, got %d", client.generateReportCalls)
	}
	if client.outcomeID != "9001" {
		t.Fatalf("outcome id = %q, want 9001", client.outcomeID)
	}
}

func TestHandleAssessmentEvaluated_ReportFailureWithoutFailedStatusReturnsError(t *testing.T) {
	client := &reportGeneratingInternalClient{
		generateReportResp: &pb.GenerateReportFromAssessmentResponse{
			Success: false,
			Status:  "skipped",
			Message: "temporary unavailable",
		},
	}
	deps := &Dependencies{
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		InternalClient: client,
	}
	handler := handleAssessmentEvaluated(deps)

	err := handler(context.Background(), "assessment.evaluated", mustBuildAssessmentEvaluatedPayload(t, 2002))
	if err == nil {
		t.Fatal("expected retryable report generation error")
	}
}
