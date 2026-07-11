package handlers

import (
	"context"
	"io"
	"log/slog"
	"testing"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/internalapi"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
)

type reportGeneratingInternalClient struct {
	fakeWorkerInternalClient
	generateReportResp *pb.GenerateReportFromAssessmentResponse
	generateReportErr  error
	outcomeID          string
}

func (f *reportGeneratingInternalClient) GenerateReportFromOutcome(_ context.Context, outcomeID string) (*pb.GenerateReportFromAssessmentResponse, error) {
	f.generateReportCalls++
	f.outcomeID = outcomeID
	if f.generateReportErr != nil {
		return nil, f.generateReportErr
	}
	return f.generateReportResp, nil
}

func TestHandleEvaluationOutcomeCommittedAcksPersistedReportFailure(t *testing.T) {
	client := &reportGeneratingInternalClient{generateReportResp: &pb.GenerateReportFromAssessmentResponse{Success: false, Status: "failed", Message: "report generation failed"}}
	handler := handleEvaluationOutcomeCommitted(&Dependencies{Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), InternalClient: client})
	if err := handler(context.Background(), eventcatalog.EvaluationOutcomeCommitted, mustBuildEvaluationOutcomeCommittedPayload(t, 2001)); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if client.generateReportCalls != 1 || client.outcomeID != "9001" {
		t.Fatalf("calls=%d outcome=%q", client.generateReportCalls, client.outcomeID)
	}
}

func TestHandleEvaluationOutcomeCommittedRetriesTransientReportFailure(t *testing.T) {
	client := &reportGeneratingInternalClient{generateReportResp: &pb.GenerateReportFromAssessmentResponse{Success: false, Status: "skipped", Message: "temporary unavailable"}}
	handler := handleEvaluationOutcomeCommitted(&Dependencies{Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), InternalClient: client})
	if err := handler(context.Background(), eventcatalog.EvaluationOutcomeCommitted, mustBuildEvaluationOutcomeCommittedPayload(t, 2002)); err == nil {
		t.Fatal("expected retryable report generation error")
	}
}
