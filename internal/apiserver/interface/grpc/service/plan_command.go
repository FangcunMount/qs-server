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
		TriggerTime:   req.GetTriggerTime(),
		Interval:      int(req.GetInterval()),
		TotalTimes:    int(req.GetTotalTimes()),
		FixedDates:    req.GetFixedDates(),
		RelativeWeeks: toIntSlice(req.GetRelativeWeeks()),
	})
	if err != nil {
		return nil, toPlanCommandGRPCError(err)
	}

	plan, convErr := toPBPlanResult(result)
	if convErr != nil {
		return nil, convErr
	}
	return &pb.CreatePlanResponse{Plan: plan}, nil
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
	plan, convErr := toPBPlanResult(result)
	if convErr != nil {
		return nil, convErr
	}
	return &pb.PausePlanResponse{Plan: plan}, nil
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
	plan, convErr := toPBPlanResult(result)
	if convErr != nil {
		return nil, convErr
	}
	return &pb.ResumePlanResponse{Plan: plan}, nil
}

func (s *PlanCommandService) FinishPlan(ctx context.Context, req *pb.FinishPlanRequest) (*pb.FinishPlanResponse, error) {
	if req.GetOrgId() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "org_id 不能为空")
	}
	if req.GetPlanId() == "" {
		return nil, status.Error(codes.InvalidArgument, "plan_id 不能为空")
	}

	result, err := s.commandService.FinishPlan(ctx, req.GetOrgId(), req.GetPlanId())
	if err != nil {
		return nil, toPlanCommandGRPCError(err)
	}
	plan, convErr := toPBPlanResult(result)
	if convErr != nil {
		return nil, convErr
	}
	return &pb.FinishPlanResponse{Plan: plan}, nil
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
	affectedTaskCount, convErr := protoInt32FromInt("affected_task_count", result.AffectedTaskCount)
	if convErr != nil {
		return nil, convErr
	}
	return &pb.CancelPlanResponse{
		PlanId:            result.PlanID,
		AffectedTaskCount: affectedTaskCount,
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
	enrollment, convErr := toPBEnrollmentResult(result)
	if convErr != nil {
		return nil, convErr
	}
	return &pb.EnrollTesteeResponse{Enrollment: enrollment}, nil
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
	affectedTaskCount, convErr := protoInt32FromInt("affected_task_count", result.AffectedTaskCount)
	if convErr != nil {
		return nil, convErr
	}
	return &pb.TerminateEnrollmentResponse{
		PlanId:            result.PlanID,
		TesteeId:          result.TesteeID,
		AffectedTaskCount: affectedTaskCount,
	}, nil
}

func (s *PlanCommandService) SchedulePendingTasks(ctx context.Context, req *pb.SchedulePendingTasksRequest) (*pb.SchedulePendingTasksResponse, error) {
	if req.GetOrgId() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "org_id 不能为空")
	}

	result, err := s.commandService.SchedulePendingTasks(ctx, req.GetOrgId(), req.GetBefore())
	if err != nil {
		return nil, toPlanCommandGRPCError(err)
	}
	tasks, convErr := toPBTaskResults(result.Tasks)
	if convErr != nil {
		return nil, convErr
	}
	stats, convErr := toPBTaskScheduleStats(result.Stats)
	if convErr != nil {
		return nil, convErr
	}
	return &pb.SchedulePendingTasksResponse{
		Tasks: tasks,
		Stats: stats,
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
	task, convErr := toPBTaskResult(result)
	if convErr != nil {
		return nil, convErr
	}
	return &pb.OpenTaskResponse{Task: task}, nil
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
	task, convErr := toPBTaskResult(result)
	if convErr != nil {
		return nil, convErr
	}
	return &pb.CompleteTaskResponse{Task: task}, nil
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
	task, convErr := toPBTaskResult(result)
	if convErr != nil {
		return nil, convErr
	}
	return &pb.ExpireTaskResponse{Task: task}, nil
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
	affectedTaskCount, convErr := protoInt32FromInt("affected_task_count", result.AffectedTaskCount)
	if convErr != nil {
		return nil, convErr
	}
	return &pb.CancelTaskResponse{
		TaskId:            result.TaskID,
		PlanId:            result.PlanID,
		AffectedTaskCount: affectedTaskCount,
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

func toPBPlanResult(result *planApp.PlanResult) (*pb.PlanResultMessage, error) {
	if result == nil {
		return nil, nil
	}
	interval, err := protoInt32FromInt("interval", result.Interval)
	if err != nil {
		return nil, err
	}
	totalTimes, err := protoInt32FromInt("total_times", result.TotalTimes)
	if err != nil {
		return nil, err
	}
	relativeWeeks, err := protoInt32Slice("relative_weeks", result.RelativeWeeks)
	if err != nil {
		return nil, err
	}

	return &pb.PlanResultMessage{
		Id:            result.ID,
		OrgId:         result.OrgID,
		ScaleCode:     result.ScaleCode,
		ScheduleType:  result.ScheduleType,
		TriggerTime:   result.TriggerTime,
		Interval:      interval,
		TotalTimes:    totalTimes,
		FixedDates:    result.FixedDates,
		RelativeWeeks: relativeWeeks,
		Status:        result.Status,
	}, nil
}

func toPBTaskResult(result *planApp.TaskResult) (*pb.TaskResultMessage, error) {
	if result == nil {
		return nil, nil
	}
	seq, err := protoInt32FromInt("seq", result.Seq)
	if err != nil {
		return nil, err
	}

	return &pb.TaskResultMessage{
		Id:           result.ID,
		PlanId:       result.PlanID,
		Seq:          seq,
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
	}, nil
}

func toPBTaskResults(results []*planApp.TaskResult) ([]*pb.TaskResultMessage, error) {
	if len(results) == 0 {
		return nil, nil
	}

	items := make([]*pb.TaskResultMessage, 0, len(results))
	for _, result := range results {
		item, err := toPBTaskResult(result)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func toPBEnrollmentResult(result *planApp.EnrollmentResult) (*pb.EnrollmentResultMessage, error) {
	if result == nil {
		return nil, nil
	}
	tasks, err := toPBTaskResults(result.Tasks)
	if err != nil {
		return nil, err
	}
	createdTaskCount, err := protoInt32FromInt("created_task_count", result.CreatedTaskCount)
	if err != nil {
		return nil, err
	}
	return &pb.EnrollmentResultMessage{
		PlanId:           result.PlanID,
		Tasks:            tasks,
		Idempotent:       result.Idempotent,
		CreatedTaskCount: createdTaskCount,
	}, nil
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

func toPBTaskScheduleStats(stats planApp.TaskScheduleStats) (*pb.TaskScheduleStatsMessage, error) {
	pendingCount, err := protoInt32FromInt("pending_count", stats.PendingCount)
	if err != nil {
		return nil, err
	}
	openedCount, err := protoInt32FromInt("opened_count", stats.OpenedCount)
	if err != nil {
		return nil, err
	}
	failedCount, err := protoInt32FromInt("failed_count", stats.FailedCount)
	if err != nil {
		return nil, err
	}
	expiredCount, err := protoInt32FromInt("expired_count", stats.ExpiredCount)
	if err != nil {
		return nil, err
	}
	expireFailedCount, err := protoInt32FromInt("expire_failed_count", stats.ExpireFailedCount)
	if err != nil {
		return nil, err
	}

	return &pb.TaskScheduleStatsMessage{
		PendingCount:      pendingCount,
		OpenedCount:       openedCount,
		FailedCount:       failedCount,
		ExpiredCount:      expiredCount,
		ExpireFailedCount: expireFailedCount,
	}, nil
}

func cloneOptionalString(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
