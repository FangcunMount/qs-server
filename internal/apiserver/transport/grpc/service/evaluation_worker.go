package service

import (
	"context"

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
	result, err := s.service.Execute(ctx, evaluationworker.Command{AssessmentID: req.AssessmentId})
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	resp := &pb.ExecuteEvaluationResponse{Status: result.Status, Retryable: result.Retryable, RunId: result.RunID, FailureKind: result.FailureKind, FailureMessage: result.FailureMessage, TraceId: result.TraceID, InputSnapshotRef: result.InputSnapshotRef}
	if result.Outcome != nil {
		resp.OutcomeId = result.Outcome.ID
		resp.Model = &pb.ModelIdentity{Kind: result.Outcome.ModelKind, SubKind: result.Outcome.SubKind, Algorithm: result.Outcome.Algorithm, Code: result.Outcome.ModelCode, Version: result.Outcome.Version, Title: result.Outcome.Title}
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
