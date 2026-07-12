package service

import (
	"context"
	"fmt"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/internalapi"
	runqueryApp "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runquery"
	interpretationautomation "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/automation"
)

func evaluateFailureResponse(
	ctx context.Context,
	runQuery runqueryApp.Service,
	assessmentID uint64,
	message string,
) *pb.EvaluateAssessmentResponse {
	resp := &pb.EvaluateAssessmentResponse{
		Success: false,
		Status:  "failed",
		Message: message,
	}
	applyLatestRunFailureMetadata(ctx, runQuery, assessmentID, func(retryable bool, runID, failureKind, traceID, inputSnapshotRef string) {
		resp.Retryable = retryable
		resp.RunId = runID
		resp.FailureKind = failureKind
		resp.TraceId = traceID
		resp.InputSnapshotRef = inputSnapshotRef
	})
	return resp
}

func generateReportFailureResponse(err error) *pb.GenerateReportFromAssessmentResponse {
	resp := &pb.GenerateReportFromAssessmentResponse{
		Success:     false,
		Status:      "failed",
		Message:     "报告生成失败",
		Retryable:   true,
		FailureKind: "internal",
	}
	if failed, ok := interpretationautomation.FailureFrom(err); ok {
		resp.Retryable = failed.Retryable
		resp.GenerationId = failed.GenerationID.String()
		resp.RunId = failed.RunID.String()
		resp.FailureKind = string(failed.Kind)
		resp.FailureCode = failed.Code
		resp.Message = failed.SafeMessage
	}
	if err != nil && resp.Message == "报告生成失败" {
		resp.Message = fmt.Sprintf("报告生成失败: %v", err)
	}
	return resp
}
