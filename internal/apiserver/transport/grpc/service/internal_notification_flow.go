package service

import (
	"context"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/FangcunMount/component-base/pkg/logger"
	notificationApp "github.com/FangcunMount/qs-server/internal/apiserver/application/notification"
	pb "github.com/FangcunMount/qs-server/internal/apiserver/interface/grpc/proto/internalapi"
)

type notificationFlow struct {
	service *InternalService
}

func newNotificationFlow(service *InternalService) notificationFlow {
	return notificationFlow{service: service}
}

func (flow notificationFlow) GenerateQuestionnaireQRCode(
	ctx context.Context,
	req *pb.GenerateQuestionnaireQRCodeRequest,
) (*pb.GenerateQuestionnaireQRCodeResponse, error) {
	return flow.service.generateQuestionnaireQRCode(ctx, req)
}

func (flow notificationFlow) HandleQuestionnairePublishedPostActions(
	ctx context.Context,
	req *pb.GenerateQuestionnaireQRCodeRequest,
) (*pb.GenerateQuestionnaireQRCodeResponse, error) {
	resp, err := flow.service.generateQuestionnaireQRCode(ctx, req)
	if err != nil {
		return nil, err
	}
	if flow.service.warmupCoordinator != nil {
		if warmErr := flow.service.warmupCoordinator.HandleQuestionnairePublished(ctx, req.GetCode(), req.GetVersion()); warmErr != nil {
			logger.L(ctx).Warnw("questionnaire publish post-actions warmup failed",
				"code", req.GetCode(),
				"version", req.GetVersion(),
				"error", warmErr,
			)
		}
	}
	return resp, nil
}

func (flow notificationFlow) GenerateScaleQRCode(
	ctx context.Context,
	req *pb.GenerateScaleQRCodeRequest,
) (*pb.GenerateScaleQRCodeResponse, error) {
	return flow.service.generateScaleQRCode(ctx, req)
}

func (flow notificationFlow) HandleScalePublishedPostActions(
	ctx context.Context,
	req *pb.GenerateScaleQRCodeRequest,
) (*pb.GenerateScaleQRCodeResponse, error) {
	resp, err := flow.service.generateScaleQRCode(ctx, req)
	if err != nil {
		return nil, err
	}
	if flow.service.warmupCoordinator != nil {
		if warmErr := flow.service.warmupCoordinator.HandleScalePublished(ctx, req.GetCode()); warmErr != nil {
			logger.L(ctx).Warnw("scale publish post-actions warmup failed",
				"code", req.GetCode(),
				"error", warmErr,
			)
		}
	}
	return resp, nil
}

func (flow notificationFlow) SendTaskOpenedMiniProgramNotification(
	ctx context.Context,
	req *pb.SendTaskOpenedMiniProgramNotificationRequest,
) (*pb.SendTaskOpenedMiniProgramNotificationResponse, error) {
	s := flow.service
	l := logger.L(ctx)

	l.Infow("gRPC: 收到 task.opened 小程序通知请求",
		"action", "send_task_opened_mini_program_notification",
		"task_id", req.GetTaskId(),
		"testee_id", req.GetTesteeId(),
	)

	if s.miniProgramTaskNotificationService == nil {
		l.Warnw("小程序 task 通知服务未配置",
			"action", "send_task_opened_mini_program_notification",
			"task_id", req.GetTaskId(),
		)
		return &pb.SendTaskOpenedMiniProgramNotificationResponse{
			Success: false,
			Skipped: true,
			Message: "小程序 task 通知服务未配置",
		}, nil
	}
	if req.GetTaskId() == "" || req.GetTesteeId() == 0 {
		return nil, status.Error(codes.InvalidArgument, "task_id 和 testee_id 不能为空")
	}

	openAt := time.Time{}
	if req.GetOpenAt() != nil {
		openAt = req.GetOpenAt().AsTime()
	}

	result, err := s.miniProgramTaskNotificationService.SendTaskOpened(ctx, notificationApp.TaskOpenedDTO{
		OrgID:    req.GetOrgId(),
		TaskID:   req.GetTaskId(),
		TesteeID: req.GetTesteeId(),
		EntryURL: req.GetEntryUrl(),
		OpenAt:   openAt,
	})
	if err != nil {
		l.Errorw("发送 task.opened 小程序通知失败",
			"action", "send_task_opened_mini_program_notification",
			"task_id", req.GetTaskId(),
			"testee_id", req.GetTesteeId(),
			"error", err.Error(),
		)
		return &pb.SendTaskOpenedMiniProgramNotificationResponse{
			Success: false,
			Message: err.Error(),
		}, nil
	}

	l.Infow("发送 task.opened 小程序通知完成",
		"action", "send_task_opened_mini_program_notification",
		"task_id", req.GetTaskId(),
		"testee_id", req.GetTesteeId(),
		"sent_count", result.SentCount,
		"skipped", result.Skipped,
		"recipient_source", result.RecipientSource,
		"recipient_open_ids", strings.Join(result.RecipientOpenIDs, ","),
		"message", result.Message,
	)

	sentCount, err := protoInt32FromInt("sent_count", result.SentCount)
	if err != nil {
		return nil, err
	}
	return &pb.SendTaskOpenedMiniProgramNotificationResponse{
		Success:          result.SentCount > 0,
		SentCount:        sentCount,
		RecipientOpenIds: result.RecipientOpenIDs,
		RecipientSource:  result.RecipientSource,
		Skipped:          result.Skipped,
		Message:          result.Message,
	}, nil
}
