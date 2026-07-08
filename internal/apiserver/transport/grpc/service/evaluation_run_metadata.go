package service

import (
	"context"

	runqueryApp "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runquery"
)

type evaluationRunMetadata struct {
	Retryable        bool
	RunID            string
	FailureKind      string
	TraceID          string
	InputSnapshotRef string
}

func latestEvaluationRunMetadata(
	ctx context.Context,
	runQuery runqueryApp.Service,
	assessmentID uint64,
) (evaluationRunMetadata, bool) {
	if runQuery == nil || assessmentID == 0 {
		return evaluationRunMetadata{}, false
	}
	run, err := runQuery.FindLatestByAssessmentID(ctx, assessmentID)
	if err != nil || run == nil {
		return evaluationRunMetadata{}, false
	}
	return evaluationRunMetadata{
		Retryable:        run.Retryable,
		RunID:            run.RunID,
		FailureKind:      run.ErrorCode,
		TraceID:          run.TraceID,
		InputSnapshotRef: run.InputSnapshotRef,
	}, true
}

func applyLatestRunFailureMetadata(
	ctx context.Context,
	runQuery runqueryApp.Service,
	assessmentID uint64,
	apply func(retryable bool, runID, failureKind, traceID, inputSnapshotRef string),
) {
	if apply == nil {
		return
	}
	meta, ok := latestEvaluationRunMetadata(ctx, runQuery, assessmentID)
	if !ok {
		return
	}
	apply(meta.Retryable, meta.RunID, meta.FailureKind, meta.TraceID, meta.InputSnapshotRef)
}

func applyLatestRunAuditMetadata(
	ctx context.Context,
	runQuery runqueryApp.Service,
	assessmentID uint64,
	apply func(traceID, inputSnapshotRef string),
) {
	if apply == nil {
		return
	}
	meta, ok := latestEvaluationRunMetadata(ctx, runQuery, assessmentID)
	if !ok {
		return
	}
	apply(meta.TraceID, meta.InputSnapshotRef)
}
