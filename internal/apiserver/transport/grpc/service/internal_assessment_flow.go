package service

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/FangcunMount/component-base/pkg/logger"
	pb "github.com/FangcunMount/qs-server/api/grpc/gen/internalapi"
	statisticsApp "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
)

type behaviorProjectionFlow struct{ service *InternalService }
type assessmentFlow struct{ service *InternalService }

func newBehaviorProjectionFlow(service *InternalService) behaviorProjectionFlow {
	return behaviorProjectionFlow{service: service}
}
func newAssessmentFlow(service *InternalService) assessmentFlow {
	return assessmentFlow{service: service}
}

func (flow behaviorProjectionFlow) ProjectBehaviorEvent(ctx context.Context, req *pb.ProjectBehaviorEventRequest) (*pb.ProjectBehaviorEventResponse, error) {
	s := flow.service
	if s.behaviorProjectorService == nil {
		return nil, status.Error(codes.FailedPrecondition, "behavior projector service is not available")
	}
	if req == nil || req.EventId == "" || req.EventType == "" || req.OccurredAt == nil {
		return nil, status.Error(codes.InvalidArgument, "event_id, event_type, org_id and occurred_at are required")
	}
	orgID, err := requestPlanOrgID(ctx, req.OrgId)
	if err != nil {
		return nil, err
	}
	result, err := s.behaviorProjectorService.ProjectBehaviorEvent(ctx, statisticsApp.BehaviorProjectEventInput{EventID: req.EventId, EventType: req.EventType, OrgID: orgID, ClinicianID: req.ClinicianId, SourceClinicianID: req.SourceClinicianId, EntryID: req.EntryId, TesteeID: req.TesteeId, AnswerSheetID: req.AnswersheetId, AssessmentID: req.AssessmentId, ReportID: req.ReportId, FailureReason: req.FailureReason, OccurredAt: req.OccurredAt.AsTime()})
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &pb.ProjectBehaviorEventResponse{Status: string(result.Status), Message: "ok"}, nil
}

func (flow assessmentFlow) SyncAssessmentAttention(ctx context.Context, req *pb.SyncAssessmentAttentionRequest) (*pb.SyncAssessmentAttentionResponse, error) {
	s := flow.service
	l := logger.L(ctx)
	if req == nil || req.TesteeId == 0 {
		return nil, status.Error(codes.InvalidArgument, "testee_id 不能为空")
	}
	result, err := s.assessmentAttentionService.SyncAssessmentAttention(ctx, req.TesteeId, req.RiskLevel, req.MarkKeyFocus)
	if err != nil {
		l.Errorw("同步测评后置关注失败", "testee_id", req.TesteeId, "error", err.Error())
		return nil, status.Errorf(codes.Internal, "同步测评后置关注失败: %v", err)
	}
	return &pb.SyncAssessmentAttentionResponse{Success: true, KeyFocusMarked: result.KeyFocusMarked, Message: "测评后置关注同步完成"}, nil
}

func interpretationTraceID(ctx context.Context) string {
	values, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	items := values.Get("x-event-id")
	if len(items) == 0 {
		return ""
	}
	return items[0]
}
