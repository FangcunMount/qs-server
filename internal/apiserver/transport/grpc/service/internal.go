package service

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/FangcunMount/component-base/pkg/logger"
	pb "github.com/FangcunMount/qs-server/api/grpc/gen/internalapi"
	operatorApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/operator"
	testeeApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/testee"
	notificationApp "github.com/FangcunMount/qs-server/internal/apiserver/application/notification"
	statisticsApp "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
)

// InternalService 内部 gRPC 服务 - 供 Worker 调用
// 用于事件处理后的业务逻辑调用。
type InternalService struct {
	pb.UnimplementedInternalServiceServer
	assessmentAttentionService testeeApp.TesteeAssessmentAttentionService
	operatorLifecycleService   operatorApp.OperatorLifecycleService
	operatorAuthService        operatorApp.OperatorAuthorizationService
	operatorQueryService       operatorApp.OperatorQueryService
	operatorRoleSyncer         operatorBootstrapRoleSyncer
	behaviorProjectorService   statisticsApp.BehaviorProjectorService
	warmupCoordinator          statisticsApp.WarmupCoordinator
	// 小程序码生成服务（可选）
	qrCodeService surveyScaleQRCodeGenerator
	// 小程序 task 消息服务（可选）
	miniProgramTaskNotificationService notificationApp.MiniProgramTaskNotificationService
}

type surveyScaleQRCodeGenerator interface {
	GenerateQuestionnaireQRCode(ctx context.Context, code, version string) (string, error)
	GenerateScaleQRCode(ctx context.Context, code string) (string, error)
}

type operatorBootstrapRoleSyncer interface {
	SyncRoles(ctx context.Context, orgID int64, operatorID uint64) error
}

// NewInternalService 创建内部 gRPC 服务
func NewInternalService(
	assessmentAttentionService testeeApp.TesteeAssessmentAttentionService,
	operatorLifecycleService operatorApp.OperatorLifecycleService,
	operatorAuthService operatorApp.OperatorAuthorizationService,
	operatorQueryService operatorApp.OperatorQueryService,
	operatorRoleSyncer operatorBootstrapRoleSyncer,
	behaviorProjectorService statisticsApp.BehaviorProjectorService,
	warmupCoordinator statisticsApp.WarmupCoordinator,
	qrCodeService surveyScaleQRCodeGenerator,
	miniProgramTaskNotificationService notificationApp.MiniProgramTaskNotificationService,
) *InternalService {
	return &InternalService{
		assessmentAttentionService:         assessmentAttentionService,
		operatorLifecycleService:           operatorLifecycleService,
		operatorAuthService:                operatorAuthService,
		operatorQueryService:               operatorQueryService,
		operatorRoleSyncer:                 operatorRoleSyncer,
		behaviorProjectorService:           behaviorProjectorService,
		warmupCoordinator:                  warmupCoordinator,
		qrCodeService:                      qrCodeService,
		miniProgramTaskNotificationService: miniProgramTaskNotificationService,
	}
}

// RegisterService 注册 gRPC 服务
func (s *InternalService) RegisterService(server *grpc.Server) {
	pb.RegisterInternalServiceServer(server, s)
}

func (s *InternalService) ProjectBehaviorEvent(
	ctx context.Context,
	req *pb.ProjectBehaviorEventRequest,
) (*pb.ProjectBehaviorEventResponse, error) {
	return newBehaviorProjectionFlow(s).ProjectBehaviorEvent(ctx, req)
}

// SyncAssessmentAttention 同步测评后置关注状态
// 场景：worker 处理 report.generated 事件后调用
// 职责：协议转换，将 gRPC 请求转换为应用服务调用
func (s *InternalService) SyncAssessmentAttention(
	ctx context.Context,
	req *pb.SyncAssessmentAttentionRequest,
) (*pb.SyncAssessmentAttentionResponse, error) {
	return newAssessmentFlow(s).SyncAssessmentAttention(ctx, req)
}

// ==================== 小程序码生成操作 ====================

// GenerateQuestionnaireQRCode 生成问卷小程序码
// 场景：worker 处理 questionnaire.changed(published) 事件后调用
func (s *InternalService) GenerateQuestionnaireQRCode(
	ctx context.Context,
	req *pb.GenerateQuestionnaireQRCodeRequest,
) (*pb.GenerateQuestionnaireQRCodeResponse, error) {
	return newNotificationFlow(s).GenerateQuestionnaireQRCode(ctx, req)
}

func (s *InternalService) HandleQuestionnairePublishedPostActions(
	ctx context.Context,
	req *pb.GenerateQuestionnaireQRCodeRequest,
) (*pb.GenerateQuestionnaireQRCodeResponse, error) {
	return newNotificationFlow(s).HandleQuestionnairePublishedPostActions(ctx, req)
}

func (s *InternalService) generateQuestionnaireQRCode(
	ctx context.Context,
	req *pb.GenerateQuestionnaireQRCodeRequest,
) (*pb.GenerateQuestionnaireQRCodeResponse, error) {
	l := logger.L(ctx)

	l.Infow("gRPC: 收到生成问卷小程序码请求",
		"action", "generate_questionnaire_qrcode",
		"code", req.Code,
		"version", req.Version,
	)

	// 验证参数
	if req.Code == "" {
		return nil, status.Error(codes.InvalidArgument, "code 不能为空")
	}
	if req.Version == "" {
		return nil, status.Error(codes.InvalidArgument, "version 不能为空")
	}

	// 检查小程序码生成服务是否配置
	if s.qrCodeService == nil {
		l.Warnw("小程序码生成服务未配置",
			"action", "generate_questionnaire_qrcode",
		)
		return &pb.GenerateQuestionnaireQRCodeResponse{
			Success: false,
			Message: "小程序码生成功能未配置",
		}, nil
	}

	// 调用应用层服务生成小程序码
	qrCodeURL, err := s.qrCodeService.GenerateQuestionnaireQRCode(ctx, req.Code, req.Version)
	if err != nil {
		l.Errorw("生成问卷小程序码失败",
			"action", "generate_questionnaire_qrcode",
			"code", req.Code,
			"version", req.Version,
			"error", err.Error(),
		)
		return &pb.GenerateQuestionnaireQRCodeResponse{
			Success: false,
			Message: err.Error(),
		}, nil
	}

	l.Infow("问卷小程序码生成成功",
		"action", "generate_questionnaire_qrcode",
		"code", req.Code,
		"version", req.Version,
		"qrcode_url", qrCodeURL,
	)

	return &pb.GenerateQuestionnaireQRCodeResponse{
		Success:   true,
		QrcodeUrl: qrCodeURL,
		Message:   "小程序码生成成功",
	}, nil
}

// GenerateScaleQRCode 生成量表小程序码
// 场景：worker 处理 assessment_model.changed(published) 事件后调用
func (s *InternalService) GenerateScaleQRCode(
	ctx context.Context,
	req *pb.GenerateScaleQRCodeRequest,
) (*pb.GenerateScaleQRCodeResponse, error) {
	return newNotificationFlow(s).GenerateScaleQRCode(ctx, req)
}

func (s *InternalService) HandleScalePublishedPostActions(
	ctx context.Context,
	req *pb.GenerateScaleQRCodeRequest,
) (*pb.GenerateScaleQRCodeResponse, error) {
	return newNotificationFlow(s).HandleScalePublishedPostActions(ctx, req)
}

func (s *InternalService) generateScaleQRCode(
	ctx context.Context,
	req *pb.GenerateScaleQRCodeRequest,
) (*pb.GenerateScaleQRCodeResponse, error) {
	l := logger.L(ctx)

	l.Infow("gRPC: 收到生成量表小程序码请求",
		"action", "generate_scale_qrcode",
		"code", req.Code,
	)

	// 验证参数
	if req.Code == "" {
		return nil, status.Error(codes.InvalidArgument, "code 不能为空")
	}

	// 检查小程序码生成服务是否配置
	if s.qrCodeService == nil {
		l.Warnw("小程序码生成服务未配置",
			"action", "generate_scale_qrcode",
		)
		return &pb.GenerateScaleQRCodeResponse{
			Success: false,
			Message: "小程序码生成功能未配置",
		}, nil
	}

	// 调用应用层服务生成小程序码
	qrCodeURL, err := s.qrCodeService.GenerateScaleQRCode(ctx, req.Code)
	if err != nil {
		l.Errorw("生成量表小程序码失败",
			"action", "generate_scale_qrcode",
			"code", req.Code,
			"error", err.Error(),
		)
		return &pb.GenerateScaleQRCodeResponse{
			Success: false,
			Message: err.Error(),
		}, nil
	}

	l.Infow("量表小程序码生成成功",
		"action", "generate_scale_qrcode",
		"code", req.Code,
		"qrcode_url", qrCodeURL,
	)

	return &pb.GenerateScaleQRCodeResponse{
		Success:   true,
		QrcodeUrl: qrCodeURL,
		Message:   "小程序码生成成功",
	}, nil
}

// SendTaskOpenedMiniProgramNotification 发送 task.opened 小程序订阅消息。
func (s *InternalService) SendTaskOpenedMiniProgramNotification(
	ctx context.Context,
	req *pb.SendTaskOpenedMiniProgramNotificationRequest,
) (*pb.SendTaskOpenedMiniProgramNotificationResponse, error) {
	return newNotificationFlow(s).SendTaskOpenedMiniProgramNotification(ctx, req)
}

// BootstrapOperator 自举首个操作者。
func (s *InternalService) BootstrapOperator(
	ctx context.Context,
	req *pb.BootstrapOperatorRequest,
) (*pb.BootstrapOperatorResponse, error) {
	return newOperatorBootstrapFlow(s).BootstrapOperator(ctx, req)
}
