package handlers

import (
	"fmt"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/internalapi"
)

func handleEvaluateAssessmentResponse(resp *pb.EvaluateAssessmentResponse) error {
	if resp == nil {
		return fmt.Errorf("evaluate assessment returned nil response")
	}
	if resp.Success {
		if resp.Status == "evaluated" {
			return nil
		}
		return fmt.Errorf("evaluate assessment returned unexpected success status: %s", resp.Status)
	}
	if resp.GetRetryable() {
		return fmt.Errorf(
			"evaluate assessment retryable failure: status=%s message=%s run_id=%s failure_kind=%s",
			resp.Status, resp.Message, resp.GetRunId(), resp.GetFailureKind(),
		)
	}
	if isTerminalEvaluateStatus(resp.Status) {
		return nil
	}
	// Fallback for proto clients that do not populate retryable yet.
	return fmt.Errorf("evaluate assessment retryable failure: status=%s message=%s", resp.Status, resp.Message)
}

func isTerminalEvaluateStatus(status string) bool {
	return status == "failed" || status == "already_evaluated"
}

func handleGenerateReportResponse(resp *pb.GenerateReportFromAssessmentResponse) error {
	if resp == nil {
		return fmt.Errorf("generate report returned nil response")
	}
	if resp.Success {
		return nil
	}
	if resp.GetRetryable() {
		return fmt.Errorf(
			"generate report retryable failure: status=%s message=%s run_id=%s failure_kind=%s",
			resp.Status, resp.Message, resp.GetRunId(), resp.GetFailureKind(),
		)
	}
	if isTerminalReportGenerationStatus(resp.Status) {
		return nil
	}
	return fmt.Errorf("generate report retryable failure: status=%s message=%s", resp.Status, resp.Message)
}

func isTerminalReportGenerationStatus(status string) bool {
	if status == "failed" {
		return true
	}
	return status == "already_generated"
}
