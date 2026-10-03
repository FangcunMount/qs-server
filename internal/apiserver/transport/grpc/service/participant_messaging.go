package service

import (
	"context"
	"errors"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/interpretation"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *ParticipantAIExplanationService) readOperation(ctx context.Context, actor app.Actor, r *pb.GetAIWorkflowRequest) (*pb.AIWorkflowResult, error) {
	o, err := s.Workflow.ReadOperation(ctx, actor, r.TesteeId, r.AssessmentId, r.RequestId, r.CommandId)
	if errors.Is(err, app.ErrNotFound) {
		return nil, status.Error(codes.NotFound, "operation not found")
	}
	if errors.Is(err, app.ErrInvalid) {
		return nil, status.Error(codes.InvalidArgument, "invalid operation identity")
	}
	if errors.Is(err, app.ErrManagementUnavailable) {
		return nil, status.Error(codes.Unavailable, "operation unavailable")
	}
	if err != nil {
		return nil, toAIExplanationGRPCError(err)
	}
	return &pb.AIWorkflowResult{RequestId: r.RequestId, Status: o.Status, Operation: &pb.ParticipantOperation{OperationId: o.OperationID, CommandId: o.CommandID, Status: o.Status, TransportStatus: o.TransportStatus, Decision: o.Decision, Code: o.Code, ResourceId: o.ResourceID, ReceiptJson: o.Receipt}}, nil
}
