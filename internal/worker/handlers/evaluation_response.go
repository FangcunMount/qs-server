package handlers

import (
	"fmt"

	evalpb "github.com/FangcunMount/qs-server/api/grpc/gen/evaluation"
	interpretationpb "github.com/FangcunMount/qs-server/api/grpc/gen/interpretation"
)

func handleEvaluateAssessmentResponse(resp *evalpb.ExecuteEvaluationResponse) error {
	if resp == nil {
		return fmt.Errorf("evaluate assessment returned nil response")
	}
	if resp.Status == "evaluated" {
		return nil
	}
	if resp.GetRetryDisposition() != "" && isDurablyClassifiedRetryDisposition(resp.GetRetryDisposition()) {
		return nil
	}
	if resp.GetRetryable() {
		return fmt.Errorf(
			"evaluate assessment retryable failure: status=%s message=%s run_id=%s failure_kind=%s",
			resp.Status, resp.GetFailureMessage(), resp.GetRunId(), resp.GetFailureKind(),
		)
	}
	if isTerminalEvaluateStatus(resp.Status) {
		return nil
	}
	// Fallback for proto clients that do not populate retryable yet.
	return fmt.Errorf("evaluate assessment retryable failure: status=%s message=%s", resp.Status, resp.GetFailureMessage())
}

func isDurablyClassifiedRetryDisposition(disposition string) bool {
	return disposition == "automatic" || disposition == "manual_required" || disposition == "terminal"
}

func isTerminalEvaluateStatus(status string) bool {
	return status == "failed" || status == "already_evaluated"
}

func handleGenerateReportResponse(resp *interpretationpb.GenerateReportFromAssessmentResponse) error {
	if resp == nil {
		return fmt.Errorf("generate report returned nil response")
	}
	if resp.Success {
		return nil
	}
	if resp.GetRetryDisposition() != "" && isDurablyClassifiedRetryDisposition(resp.GetRetryDisposition()) {
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
	if status == "failed" || status == "admission_rejected" {
		return true
	}
	return status == "already_generated"
}
