package service

import (
	"context"

	pkgerrors "github.com/FangcunMount/component-base/pkg/errors"
	planApp "github.com/FangcunMount/qs-server/internal/apiserver/application/plan"
	pb "github.com/FangcunMount/qs-server/internal/apiserver/interface/grpc/proto/internalapi"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type PlanCommandService struct {
	pb.UnimplementedPlanCommandServiceServer
	commandService planApp.PlanCommandService
}

func NewPlanCommandService(commandService planApp.PlanCommandService) *PlanCommandService {
	return &PlanCommandService{commandService: commandService}
}

func (s *PlanCommandService) RegisterService(server *grpc.Server) {
	pb.RegisterPlanCommandServiceServer(server, s)
}

func (s *PlanCommandService) CreatePlan(ctx context.Context, req *pb.CreatePlanRequest) (*pb.CreatePlanResponse, error) {
	if req.GetOrgId() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "org_id 不能为空")
	}
	if req.GetScaleCode() == "" {
		return nil, status.Error(codes.InvalidArgument, "scale_code 不能为空")
	}
	if req.GetScheduleType() == "" {
		return nil, status.Error(codes.InvalidArgument, "schedule_type 不能为空")
	}

	result, err := s.commandService.CreatePlan(ctx, planApp.CreatePlanDTO{
		OrgID:         req.GetOrgId(),
		ScaleCode:     req.GetScaleCode(),
		ScheduleType:  req.GetScheduleType(),
		Interval:      int(req.GetInterval()),
		TotalTimes:    int(req.GetTotalTimes()),
		FixedDates:    req.GetFixedDates(),
		RelativeWeeks: toIntSlice(req.GetRelativeWeeks()),
	})
	if err != nil {
		return nil, toPlanCommandGRPCError(err)
	}

	return &pb.CreatePlanResponse{Plan: toPBPlanResult(result)}, nil
}

func (s *PlanCommandService) PausePlan(ctx context.Context, req *pb.PausePlanRequest) (*pb.PausePlanResponse, error) {
	if req.GetOrgId() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "org_id 不能为空")
	}
	if req.GetPlanId() == "" {
		return nil, status.Error(codes.InvalidArgument, "plan_id 不能为空")
	}

	result, err := s.commandService.PausePlan(ctx, req.GetOrgId(), req.GetPlanId())
	if err != nil {
		return nil, toPlanCommandGRPCError(err)
	}
	return &pb.PausePlanResponse{Plan: toPBPlanResult(result)}, nil
}

func (s *PlanCommandService) ResumePlan(ctx context.Context, req *pb.ResumePlanRequest) (*pb.ResumePlanResponse, error) {
	if req.GetOrgId() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "org_id 不能为空")
	}
	if req.GetPlanId() == "" {
		return nil, status.Error(codes.InvalidArgument, "plan_id 不能为空")
	}

	result, err := s.commandService.ResumePlan(ctx, req.GetOrgId(), req.GetPlanId(), req.GetTesteeStartDates())
	if err != nil {
		return nil, toPlanCommandGRPCError(err)
	}
	return &pb.ResumePlanResponse{Plan: toPBPlanResult(result)}, nil
}

func (s *PlanCommandService) CancelPlan(ctx context.Context, req *pb.CancelPlanRequest) (*pb.CancelPlanResponse, error) {
	if req.GetOrgId() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "org_id 不能为空")
	}
	if req.GetPlanId() == "" {
		return nil, status.Error(codes.InvalidArgument, "plan_id 不能为空")
	}

	result, err := s.commandService.CancelPlan(ctx, req.GetOrgId(), req.GetPlanId())
	if err != nil {
		return nil, toPlanCommandGRPCError(err)
	}
	return &pb.CancelPlanResponse{
		PlanId:            result.PlanID,
		AffectedTaskCount: int32(result.AffectedTaskCount),
	}, nil
}

func (s *PlanCommandService) EnrollTestee(ctx context.Context, req *pb.EnrollTesteeRequest) (*pb.EnrollTesteeResponse, error) {
	if req.GetOrgId() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "org_id 不能为空")
	}
	if req.GetPlanId() == "" {
		return nil, status.Error(codes.InvalidArgument, "plan_id 不能为空")
	}
	if req.GetTesteeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "testee_id 不能为空")
	}
	if req.GetStartDate() == "" {
		return nil, status.Error(codes.InvalidArgument, "start_date 不能为空")
	}

	result, err := s.commandService.EnrollTestee(ctx, planApp.EnrollTesteeDTO{
		OrgID:     req.GetOrgId(),
		PlanID:    req.GetPlanId(),
		TesteeID:  req.GetTesteeId(),
		StartDate: req.GetStartDate(),
	})
	if err != nil {
		return nil, toPlanCommandGRPCError(err)
	}
	return &pb.EnrollTesteeResponse{Enrollment: toPBEnrollmentResult(result)}, nil
}

func (s *PlanCommandService) TerminateEnrollment(ctx context.Context, req *pb.TerminateEnrollmentRequest) (*pb.TerminateEnrollmentResponse, error) {
	if req.GetOrgId() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "org_id 不能为空")
	}
	if req.GetPlanId() == "" {
		return nil, status.Error(codes.InvalidArgument, "plan_id 不能为空")
	}
	if req.GetTesteeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "testee_id 不能为空")
	}

	result, err := s.commandService.TerminateEnrollment(ctx, req.GetOrgId(), req.GetPlanId(), req.GetTesteeId())
	if err != nil {
		return nil, toPlanCommandGRPCError(err)
	}
	return &pb.TerminateEnrollmentResponse{
		PlanId:            result.PlanID,
		TesteeId:          result.TesteeID,
		AffectedTaskCount: int32(result.AffectedTaskCount),
	}, nil
}

func (s *PlanCommandService) SchedulePendingTasks(ctx context.Context, req *pb.SchedulePendingTasksRequest) (*pb.SchedulePendingTasksResponse, error) {
	if req.GetOrgId() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "org_id 不能为空")
	}
	if req.GetSource() != "" {
		ctx = planApp.WithTaskSchedulerSource(ctx, req.GetSource())
	}

	result, err := s.commandService.SchedulePendingTasks(ctx, req.GetOrgId(), req.GetBefore())
	if err != nil {
		return nil, toPlanCommandGRPCError(err)
	}
	return &pb.SchedulePendingTasksResponse{
		Tasks: toPBTaskResults(result.Tasks),
		Stats: &pb.TaskScheduleStatsMessage{
			PendingCount:      int32(result.Stats.PendingCount),
			OpenedCount:       int32(result.Stats.OpenedCount),
			FailedCount:       int32(result.Stats.FailedCount),
			ExpiredCount:      int32(result.Stats.ExpiredCount),
			ExpireFailedCount: int32(result.Stats.ExpireFailedCount),
		},
	}, nil
}

func (s *PlanCommandService) OpenTask(ctx context.Context, req *pb.OpenTaskRequest) (*pb.OpenTaskResponse, error) {
	if req.GetOrgId() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "org_id 不能为空")
	}
	if req.GetTaskId() == "" {
		return nil, status.Error(codes.InvalidArgument, "task_id 不能为空")
	}

	result, err := s.commandService.OpenTask(ctx, req.GetOrgId(), req.GetTaskId(), planApp.OpenTaskDTO{
		EntryToken: req.GetEntryToken(),
		EntryURL:   req.GetEntryUrl(),
		ExpireAt:   req.GetExpireAt(),
	})
	if err != nil {
		return nil, toPlanCommandGRPCError(err)
	}
	return &pb.OpenTaskResponse{Task: toPBTaskResult(result)}, nil
}

func (s *PlanCommandService) CompleteTask(ctx context.Context, req *pb.CompleteTaskRequest) (*pb.CompleteTaskResponse, error) {
	if req.GetOrgId() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "org_id 不能为空")
	}
	if req.GetTaskId() == "" {
		return nil, status.Error(codes.InvalidArgument, "task_id 不能为空")
	}
	if req.GetAssessmentId() == "" {
		return nil, status.Error(codes.InvalidArgument, "assessment_id 不能为空")
	}

	result, err := s.commandService.CompleteTask(ctx, req.GetOrgId(), req.GetTaskId(), req.GetAssessmentId())
	if err != nil {
		return nil, toPlanCommandGRPCError(err)
	}
	return &pb.CompleteTaskResponse{Task: toPBTaskResult(result)}, nil
}

func (s *PlanCommandService) ExpireTask(ctx context.Context, req *pb.ExpireTaskRequest) (*pb.ExpireTaskResponse, error) {
	if req.GetOrgId() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "org_id 不能为空")
	}
	if req.GetTaskId() == "" {
		return nil, status.Error(codes.InvalidArgument, "task_id 不能为空")
	}

	result, err := s.commandService.ExpireTask(ctx, req.GetOrgId(), req.GetTaskId())
	if err != nil {
		return nil, toPlanCommandGRPCError(err)
	}
	return &pb.ExpireTaskResponse{Task: toPBTaskResult(result)}, nil
}

func (s *PlanCommandService) CancelTask(ctx context.Context, req *pb.CancelTaskRequest) (*pb.CancelTaskResponse, error) {
	if req.GetOrgId() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "org_id 不能为空")
	}
	if req.GetTaskId() == "" {
		return nil, status.Error(codes.InvalidArgument, "task_id 不能为空")
	}

	result, err := s.commandService.CancelTask(ctx, req.GetOrgId(), req.GetTaskId())
	if err != nil {
		return nil, toPlanCommandGRPCError(err)
	}
	return &pb.CancelTaskResponse{
		TaskId:            result.TaskID,
		PlanId:            result.PlanID,
		AffectedTaskCount: int32(result.AffectedTaskCount),
	}, nil
}

func toPlanCommandGRPCError(err error) error {
	if err == nil {
		return nil
	}

	coder := pkgerrors.ParseCoder(err)
	switch coder.Code() {
	case errorCode.ErrInvalidArgument, errorCode.ErrValidation, errorCode.ErrBind:
		return status.Error(codes.InvalidArgument, err.Error())
	case errorCode.ErrPageNotFound:
		return status.Error(codes.NotFound, err.Error())
	case errorCode.ErrPermissionDenied:
		return status.Error(codes.PermissionDenied, err.Error())
	default:
		return status.Error(codes.Internal, err.Error())
	}
}

func toPBPlanResult(result *planApp.PlanResult) *pb.PlanResultMessage {
	if result == nil {
		return nil
	}

	return &pb.PlanResultMessage{
		Id:            result.ID,
		OrgId:         result.OrgID,
		ScaleCode:     result.ScaleCode,
		ScheduleType:  result.ScheduleType,
		Interval:      int32(result.Interval),
		TotalTimes:    int32(result.TotalTimes),
		FixedDates:    result.FixedDates,
		RelativeWeeks: toInt32Slice(result.RelativeWeeks),
		Status:        result.Status,
	}
}

func toPBTaskResult(result *planApp.TaskResult) *pb.TaskResultMessage {
	if result == nil {
		return nil
	}

	return &pb.TaskResultMessage{
		Id:           result.ID,
		PlanId:       result.PlanID,
		Seq:          int32(result.Seq),
		OrgId:        result.OrgID,
		TesteeId:     result.TesteeID,
		ScaleCode:    result.ScaleCode,
		PlannedAt:    result.PlannedAt,
		OpenAt:       cloneOptionalString(result.OpenAt),
		ExpireAt:     cloneOptionalString(result.ExpireAt),
		CompletedAt:  cloneOptionalString(result.CompletedAt),
		Status:       result.Status,
		AssessmentId: cloneOptionalString(result.AssessmentID),
		EntryToken:   result.EntryToken,
		EntryUrl:     result.EntryURL,
	}
}

func toPBTaskResults(results []*planApp.TaskResult) []*pb.TaskResultMessage {
	if len(results) == 0 {
		return nil
	}

	items := make([]*pb.TaskResultMessage, 0, len(results))
	for _, result := range results {
		items = append(items, toPBTaskResult(result))
	}
	return items
}

func toPBEnrollmentResult(result *planApp.EnrollmentResult) *pb.EnrollmentResultMessage {
	if result == nil {
		return nil
	}
	return &pb.EnrollmentResultMessage{
		PlanId:           result.PlanID,
		Tasks:            toPBTaskResults(result.Tasks),
		Idempotent:       result.Idempotent,
		CreatedTaskCount: int32(result.CreatedTaskCount),
	}
}

func toIntSlice(values []int32) []int {
	if len(values) == 0 {
		return nil
	}
	items := make([]int, 0, len(values))
	for _, value := range values {
		items = append(items, int(value))
	}
	return items
}

func toInt32Slice(values []int) []int32 {
	if len(values) == 0 {
		return nil
	}
	items := make([]int32, 0, len(values))
	for _, value := range values {
		items = append(items, int32(value))
	}
	return items
}

func cloneOptionalString(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
