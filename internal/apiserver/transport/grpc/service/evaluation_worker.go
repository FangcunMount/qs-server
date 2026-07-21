package service

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/logger"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/evaluation"
	evaluationworker "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/worker"
)

type EvaluationWorkerService struct {
	pb.UnimplementedEvaluationWorkerServiceServer
	service evaluationworker.Service
}

func NewEvaluationWorkerService(service evaluationworker.Service) *EvaluationWorkerService {
	return &EvaluationWorkerService{service: service}
}
func (s *EvaluationWorkerService) RegisterService(server *grpc.Server) {
	pb.RegisterEvaluationWorkerServiceServer(server, s)
}

func (s *EvaluationWorkerService) ExecuteEvaluation(ctx context.Context, req *pb.ExecuteEvaluationRequest) (*pb.ExecuteEvaluationResponse, error) {
	if req == nil || req.AssessmentId == 0 {
		return nil, status.Error(codes.InvalidArgument, "assessment_id 不能为空")
	}
	logger.L(ctx).Infow("gRPC: received evaluation execution request", "assessment_id", req.AssessmentId)
	ctx = withRetryAuthorization(ctx)
	result, err := s.service.Execute(ctx, evaluationworker.Command{AssessmentID: req.AssessmentId})
	if err != nil {
		return nil, toEvaluationGRPCError(err)
	}
	resp := &pb.ExecuteEvaluationResponse{
		Status: result.Status, Retryable: result.Retryable, RunId: result.RunID,
		FailureKind: result.FailureKind, FailureMessage: result.FailureMessage,
		TraceId: result.TraceID, InputSnapshotRef: result.InputSnapshotRef,
		RetryDisposition: result.RetryDisposition, AttemptOrigin: result.AttemptOrigin,
		CurrentAttempt: int32(result.CurrentAttempt), MaxAutomaticAttempts: int32(result.MaxAutomaticAttempts),
		RemainingAutomaticAttempts: int32(result.RemainingAutomaticAttempts), RetryEventId: result.RetryEventID,
		ActionRequestId: result.ActionRequestID,
	}
	if result.NextAttemptAt != nil {
		resp.NextAttemptAt = result.NextAttemptAt.UTC().Format(time.RFC3339Nano)
	}
	if result.Outcome != nil {
		resp.OutcomeId = result.Outcome.ID
		resp.Model = &pb.ModelIdentity{
			Kind: result.Outcome.ModelKind, SubKind: result.Outcome.SubKind, Algorithm: result.Outcome.Algorithm,
			Code: result.Outcome.ModelCode, Version: result.Outcome.Version, Title: result.Outcome.Title,
			AlgorithmFamily: result.Outcome.AlgorithmFamily,
			DecisionKind:    result.Outcome.DecisionKind,
			PayloadFormat:   result.Outcome.PayloadFormat,
		}
		if result.Outcome.TotalScore != nil {
			resp.PrimaryScore = &pb.ScoreValue{Kind: "raw_total", Value: *result.Outcome.TotalScore}
		}
		if result.Outcome.RiskLevel != "" {
			resp.Level = &pb.ResultLevel{Code: result.Outcome.RiskLevel, Label: result.Outcome.RiskLevel}
		}
	}
	logger.L(ctx).Infow("gRPC: evaluation execution completed",
		"assessment_id", req.AssessmentId,
		"status", result.Status,
		"evaluation_run_id", result.RunID,
		"outcome_id", resp.OutcomeId,
		"retryable", result.Retryable,
	)
	return resp, nil
}
