package response

import (
	"time"

	evaluationoperator "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/operator"
)

// EvaluationRunResponse is the REST view of one evaluation run attempt.
type EvaluationRunResponse struct {
	RunID                      string     `json:"run_id"`
	AssessmentID               uint64     `json:"assessment_id"`
	AttemptNo                  int        `json:"attempt_no"`
	Status                     string     `json:"status"`
	Retryable                  bool       `json:"retryable"`
	ErrorCode                  string     `json:"error_code,omitempty"`
	ErrorMessage               string     `json:"error_message,omitempty"`
	StartedAt                  time.Time  `json:"started_at"`
	FinishedAt                 *time.Time `json:"finished_at,omitempty"`
	TraceID                    string     `json:"trace_id,omitempty"`
	InputSnapshotRef           string     `json:"input_snapshot_ref,omitempty"`
	AttemptOrigin              string     `json:"attempt_origin,omitempty"`
	RetryDisposition           string     `json:"retry_disposition,omitempty"`
	MaxAutomaticAttempts       int        `json:"max_automatic_attempts,omitempty"`
	RemainingAutomaticAttempts int        `json:"remaining_automatic_attempts,omitempty"`
	NextAttemptAt              *time.Time `json:"next_attempt_at,omitempty"`
	RetryEventID               string     `json:"retry_event_id,omitempty"`
	ActionRequestID            string     `json:"action_request_id,omitempty"`
}

// EvaluationRunListResponse lists evaluation runs for one assessment.
type EvaluationRunListResponse struct {
	Items []*EvaluationRunResponse `json:"items"`
}

// RetryableFailedRunResponse includes org scope for operating queries.
type RetryableFailedRunResponse struct {
	EvaluationRunResponse
	OrgID int64 `json:"org_id"`
}

// RetryableFailedRunListResponse is a cursor page of retryable failed runs.
type RetryableFailedRunListResponse struct {
	Items      []*RetryableFailedRunResponse `json:"items"`
	NextCursor uint64                        `json:"next_cursor,omitempty"`
}

// NewEvaluationRunResponse maps an application run result to REST.
func NewEvaluationRunResponse(result *evaluationoperator.Run) *EvaluationRunResponse {
	if result == nil {
		return nil
	}
	return &EvaluationRunResponse{
		RunID:            result.RunID,
		AssessmentID:     result.AssessmentID,
		AttemptNo:        result.AttemptNo,
		Status:           result.Status,
		Retryable:        result.Retryable,
		ErrorCode:        result.ErrorCode,
		ErrorMessage:     result.ErrorMessage,
		StartedAt:        result.StartedAt,
		FinishedAt:       result.FinishedAt,
		TraceID:          result.TraceID,
		InputSnapshotRef: result.InputSnapshotRef,
		AttemptOrigin:    result.AttemptOrigin, RetryDisposition: result.RetryDisposition,
		MaxAutomaticAttempts: result.MaxAutomaticAttempts, RemainingAutomaticAttempts: result.RemainingAutomaticAttempts,
		NextAttemptAt: result.NextAttemptAt, RetryEventID: result.RetryEventID, ActionRequestID: result.ActionRequestID,
	}
}

// NewEvaluationRunListResponse maps a protected query list to REST.
func NewEvaluationRunListResponse(result *evaluationoperator.RunList) *EvaluationRunListResponse {
	if result == nil {
		return &EvaluationRunListResponse{Items: []*EvaluationRunResponse{}}
	}
	items := make([]*EvaluationRunResponse, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, NewEvaluationRunResponse(item))
	}
	return &EvaluationRunListResponse{Items: items}
}

// NewRetryableFailedRunListResponse maps an operating query page to REST.
func NewRetryableFailedRunListResponse(result *evaluationoperator.RetryableFailedRunList) *RetryableFailedRunListResponse {
	if result == nil {
		return &RetryableFailedRunListResponse{Items: []*RetryableFailedRunResponse{}}
	}
	items := make([]*RetryableFailedRunResponse, 0, len(result.Items))
	for _, item := range result.Items {
		if item == nil {
			continue
		}
		items = append(items, &RetryableFailedRunResponse{
			EvaluationRunResponse: EvaluationRunResponse{
				RunID:            item.RunID,
				AssessmentID:     item.AssessmentID,
				AttemptNo:        item.AttemptNo,
				Status:           item.Status,
				Retryable:        item.Retryable,
				ErrorCode:        item.ErrorCode,
				ErrorMessage:     item.ErrorMessage,
				StartedAt:        item.StartedAt,
				FinishedAt:       item.FinishedAt,
				TraceID:          item.TraceID,
				InputSnapshotRef: item.InputSnapshotRef,
				AttemptOrigin:    item.AttemptOrigin, RetryDisposition: item.RetryDisposition,
				MaxAutomaticAttempts: item.MaxAutomaticAttempts, RemainingAutomaticAttempts: item.RemainingAutomaticAttempts,
				NextAttemptAt: item.NextAttemptAt, RetryEventID: item.RetryEventID, ActionRequestID: item.ActionRequestID,
			},
			OrgID: item.OrgID,
		})
	}
	return &RetryableFailedRunListResponse{
		Items:      items,
		NextCursor: result.NextCursor,
	}
}
