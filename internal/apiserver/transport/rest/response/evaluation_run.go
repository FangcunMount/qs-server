package response

import (
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/assessment"
	runquery "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runquery"
)

// EvaluationRunResponse is the REST view of one evaluation run attempt.
type EvaluationRunResponse struct {
	RunID            string     `json:"run_id"`
	AssessmentID     uint64     `json:"assessment_id"`
	AttemptNo        int        `json:"attempt_no"`
	Status           string     `json:"status"`
	Retryable        bool       `json:"retryable"`
	ErrorCode        string     `json:"error_code,omitempty"`
	ErrorMessage     string     `json:"error_message,omitempty"`
	StartedAt        time.Time  `json:"started_at"`
	FinishedAt       *time.Time `json:"finished_at,omitempty"`
	TraceID          string     `json:"trace_id,omitempty"`
	InputSnapshotRef string     `json:"input_snapshot_ref,omitempty"`
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
func NewEvaluationRunResponse(result *assessment.AssessmentRunResult) *EvaluationRunResponse {
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
	}
}

// NewEvaluationRunListResponse maps a protected query list to REST.
func NewEvaluationRunListResponse(result *assessment.AssessmentRunListResult) *EvaluationRunListResponse {
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
func NewRetryableFailedRunListResponse(result *runquery.RetryableFailedListResult) *RetryableFailedRunListResponse {
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
			},
			OrgID: item.OrgID,
		})
	}
	return &RetryableFailedRunListResponse{
		Items:      items,
		NextCursor: result.NextCursor,
	}
}
