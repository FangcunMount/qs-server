package service

import (
	"context"
	"fmt"
	"log/slog"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/interpretation"
	automation "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/automation"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type InterpretationAutomationService struct {
	pb.UnimplementedInterpretationAutomationServiceServer
	service automation.Service
}

func NewInterpretationAutomationService(service automation.Service) *InterpretationAutomationService {
	return &InterpretationAutomationService{service: service}
}
func (s *InterpretationAutomationService) RegisterService(server *grpc.Server) {
	pb.RegisterInterpretationAutomationServiceServer(server, s)
}

func (s *InterpretationAutomationService) GenerateReportFromAssessment(ctx context.Context, req *pb.GenerateReportFromAssessmentRequest) (*pb.GenerateReportFromAssessmentResponse, error) {
	if req == nil || req.OutcomeId == "" {
		return nil, status.Error(codes.InvalidArgument, "outcome_id 不能为空")
	}
	if s.service == nil {
		return generateReportFailureResponse(fmt.Errorf("interpretation automation service is not configured")), nil
	}
	outcomeID, err := meta.ParseID(req.OutcomeId)
	if err != nil || outcomeID.IsZero() {
		return nil, status.Error(codes.InvalidArgument, "outcome_id 无效")
	}
	result, err := s.service.Generate(ctx, automation.GenerateCommand{Actor: automation.TrustedServiceActor("internal-grpc"), OutcomeID: outcomeID, TraceID: interpretationTraceID(ctx)})
	if err != nil {
		slog.ErrorContext(ctx, "interpretation automation failed", "outcome_id", req.OutcomeId, "error", err)
		return generateReportFailureResponse(err), nil
	}
	statusValue, message := "generated", "报告生成完成"
	if result != nil && result.Status == automation.StatusProcessing {
		statusValue, message = "processing", "报告正在生成"
	}
	resp := &pb.GenerateReportFromAssessmentResponse{Success: true, Status: statusValue, Message: message}
	if result != nil {
		resp.GenerationId = result.GenerationID.String()
		resp.RunId = result.RunID.String()
		resp.ReportId = result.ReportID.String()
	}
	return resp, nil
}

func generateReportFailureResponse(err error) *pb.GenerateReportFromAssessmentResponse {
	resp := &pb.GenerateReportFromAssessmentResponse{Success: false, Status: "failed", Message: "报告生成失败", Retryable: true, FailureKind: "internal"}
	if failed, ok := automation.FailureFrom(err); ok {
		resp.Retryable, resp.GenerationId, resp.RunId = failed.Retryable, failed.GenerationID.String(), failed.RunID.String()
		resp.FailureKind, resp.FailureCode, resp.Message = string(failed.Kind), failed.Code, failed.SafeMessage
	}
	if err != nil && resp.Message == "报告生成失败" {
		resp.FailureCode = "internal_error"
	}
	return resp
}
