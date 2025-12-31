package service

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/FangcunMount/component-base/pkg/logger"
	testeeApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/testee"
	assessmentApp "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/engine"
	planApp "github.com/FangcunMount/qs-server/internal/apiserver/application/plan"
	qrcodeApp "github.com/FangcunMount/qs-server/internal/apiserver/application/qrcode"
	statisticsApp "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	answerSheetApp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/answersheet"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	pb "github.com/FangcunMount/qs-server/internal/apiserver/interface/grpc/proto/internalapi"
)

// InternalService 内部 gRPC 服务 - 供 Worker 和 Sync 调用
// 用于事件处理后的业务逻辑调用和定时任务调用
type InternalService struct {
	pb.UnimplementedInternalServiceServer
	answerSheetScoringService answerSheetApp.AnswerSheetScoringService
	submissionService         assessmentApp.AssessmentSubmissionService
	managementService         assessmentApp.AssessmentManagementService
	engineService             engine.Service
	scaleRepo                 scale.Repository
	testeeTaggingService      testeeApp.TesteeTaggingService
	// Statistics 服务（备用接口，推荐使用 REST API + Crontab）
	statisticsSyncService      statisticsApp.StatisticsSyncService
	statisticsValidatorService statisticsApp.StatisticsValidatorService
	// Plan 服务（备用接口，推荐使用 REST API + Crontab）
	taskSchedulerService planApp.TaskSchedulerService
	// 小程序码生成服务（可选）
	qrCodeService qrcodeApp.QRCodeService
}

// NewInternalService 创建内部 gRPC 服务
func NewInternalService(
	answerSheetScoringService answerSheetApp.AnswerSheetScoringService,
	submissionService assessmentApp.AssessmentSubmissionService,
	managementService assessmentApp.AssessmentManagementService,
	engineService engine.Service,
	scaleRepo scale.Repository,
	testeeTaggingService testeeApp.TesteeTaggingService,
	statisticsSyncService interface{}, // statisticsApp.StatisticsSyncService，可能为 nil
	statisticsValidatorService interface{}, // statisticsApp.StatisticsValidatorService，可能为 nil
	taskSchedulerService interface{}, // planApp.TaskSchedulerService，可能为 nil
	qrCodeService interface{}, // qrcodeApp.QRCodeService，可能为 nil
) *InternalService {
	// 类型转换（如果提供了服务）
	var syncService statisticsApp.StatisticsSyncService
	if s, ok := statisticsSyncService.(statisticsApp.StatisticsSyncService); ok {
		syncService = s
	}

	var validatorService statisticsApp.StatisticsValidatorService
	if v, ok := statisticsValidatorService.(statisticsApp.StatisticsValidatorService); ok {
		validatorService = v
	}

	var schedulerService planApp.TaskSchedulerService
	if t, ok := taskSchedulerService.(planApp.TaskSchedulerService); ok {
		schedulerService = t
	}

	var qrService qrcodeApp.QRCodeService
	if q, ok := qrCodeService.(qrcodeApp.QRCodeService); ok {
		qrService = q
	}

	return &InternalService{
		answerSheetScoringService:  answerSheetScoringService,
		submissionService:          submissionService,
		managementService:          managementService,
		engineService:              engineService,
		scaleRepo:                  scaleRepo,
		testeeTaggingService:       testeeTaggingService,
		statisticsSyncService:      syncService,
		statisticsValidatorService: validatorService,
		taskSchedulerService:       schedulerService,
		qrCodeService:              qrService,
	}
}

// RegisterService 注册 gRPC 服务
func (s *InternalService) RegisterService(server *grpc.Server) {
	pb.RegisterInternalServiceServer(server, s)
}

// CalculateAnswerSheetScore 计算答卷分数
// 场景：worker 处理 answersheet.submitted 事件后调用
func (s *InternalService) CalculateAnswerSheetScore(
	ctx context.Context,
	req *pb.CalculateAnswerSheetScoreRequest,
) (*pb.CalculateAnswerSheetScoreResponse, error) {
	l := logger.L(ctx)

	l.Infow("gRPC: 收到答卷计分请求",
		"action", "calculate_answersheet_score",
		"answersheet_id", req.AnswersheetId,
	)

	// 验证参数
	if req.AnswersheetId == 0 {
		return &pb.CalculateAnswerSheetScoreResponse{
			Success: false,
			Message: "answersheet_id 不能为空",
		}, nil
	}

	// 调用应用服务计算分数
	err := s.answerSheetScoringService.CalculateAndSave(ctx, req.AnswersheetId)
	if err != nil {
		l.Errorw("答卷计分失败",
			"answersheet_id", req.AnswersheetId,
			"error", err.Error(),
		)
		return &pb.CalculateAnswerSheetScoreResponse{
			Success: false,
			Message: err.Error(),
		}, nil
	}

	l.Infow("答卷计分成功",
		"answersheet_id", req.AnswersheetId,
	)

	return &pb.CalculateAnswerSheetScoreResponse{
		Success: true,
		Message: "计分成功",
	}, nil
}

// CreateAssessmentFromAnswerSheet 从答卷创建测评
// 场景：worker 处理 answersheet.submitted 事件后调用（在计分之后）
func (s *InternalService) CreateAssessmentFromAnswerSheet(
	ctx context.Context,
	req *pb.CreateAssessmentFromAnswerSheetRequest,
) (*pb.CreateAssessmentFromAnswerSheetResponse, error) {
	l := logger.L(ctx)

	l.Infow("gRPC: 收到从答卷创建测评请求",
		"action", "create_assessment_from_answersheet",
		"answersheet_id", req.AnswersheetId,
		"questionnaire_code", req.QuestionnaireCode,
		"filler_id", req.FillerId,
	)

	// 验证参数
	if req.AnswersheetId == 0 {
		return nil, status.Error(codes.InvalidArgument, "answersheet_id 不能为空")
	}
	if req.QuestionnaireCode == "" {
		return nil, status.Error(codes.InvalidArgument, "questionnaire_code 不能为空")
	}
	if req.QuestionnaireVersion == "" {
		return nil, status.Error(codes.InvalidArgument, "questionnaire_version 不能为空")
	}
	if req.TesteeId == 0 {
		return nil, status.Error(codes.InvalidArgument, "testee_id 不能为空")
	}
	if req.FillerId == 0 {
		return nil, status.Error(codes.InvalidArgument, "filler_id 不能为空")
	}

	// 查找问卷关联的量表（可能没有）
	var medicalScaleID *uint64
	var medicalScaleCode *string
	var medicalScaleName *string

	medicalScale, err := s.scaleRepo.FindByQuestionnaireCode(ctx, req.QuestionnaireCode)
	if err == nil && medicalScale != nil {
		// 找到关联的量表
		scaleID := medicalScale.GetID().Uint64()
		scaleCode := medicalScale.GetCode().Value()
		scaleName := medicalScale.GetTitle()

		medicalScaleID = &scaleID
		medicalScaleCode = &scaleCode
		medicalScaleName = &scaleName

		l.Infow("找到关联量表",
			"scale_id", scaleID,
			"scale_code", scaleCode,
			"scale_name", scaleName,
		)
	} else {
		l.Infow("问卷未关联量表，将创建纯问卷模式的测评",
			"questionnaire_code", req.QuestionnaireCode,
		)
	}

	// 构建创建 DTO（使用 QuestionnaireCode 作为唯一标识）
	dto := assessmentApp.CreateAssessmentDTO{
		OrgID:                req.OrgId,
		TesteeID:             req.TesteeId,
		QuestionnaireCode:    req.QuestionnaireCode,
		QuestionnaireVersion: req.QuestionnaireVersion,
		AnswerSheetID:        req.AnswersheetId,
		MedicalScaleID:       medicalScaleID,
		MedicalScaleCode:     medicalScaleCode,
		MedicalScaleName:     medicalScaleName,
		OriginType:           req.OriginType,
	}

	if dto.OriginType == "" {
		dto.OriginType = "adhoc"
	}
	if req.OriginId != "" {
		dto.OriginID = &req.OriginId
	}

	// 调用应用服务创建测评
	result, err := s.submissionService.Create(ctx, dto)
	if err != nil {
		l.Errorw("创建测评失败",
			"action", "create_assessment_from_answersheet",
			"result", "failed",
			"error", err.Error(),
		)
		return nil, status.Errorf(codes.Internal, "创建测评失败: %v", err)
	}

	l.Infow("创建测评成功",
		"action", "create_assessment_from_answersheet",
		"assessment_id", result.ID,
		"result", "success",
	)

	// 如果有关联量表，自动提交测评
	autoSubmitted := false
	if medicalScaleID != nil {
		_, err := s.submissionService.Submit(ctx, result.ID)
		if err != nil {
			l.Warnw("自动提交测评失败",
				"assessment_id", result.ID,
				"error", err.Error(),
			)
		} else {
			autoSubmitted = true
			l.Infow("自动提交测评成功",
				"assessment_id", result.ID,
			)
		}
	}

	return &pb.CreateAssessmentFromAnswerSheetResponse{
		AssessmentId:  result.ID,
		Created:       true,
		AutoSubmitted: autoSubmitted,
		Message:       "测评创建成功",
	}, nil
}

// EvaluateAssessment 执行测评评估
// 场景：worker 处理 assessment.submitted 事件后调用
func (s *InternalService) EvaluateAssessment(
	ctx context.Context,
	req *pb.EvaluateAssessmentRequest,
) (*pb.EvaluateAssessmentResponse, error) {
	l := logger.L(ctx)

	l.Infow("gRPC: 收到执行评估请求",
		"action", "evaluate_assessment",
		"assessment_id", req.AssessmentId,
	)

	// 验证参数
	if req.AssessmentId == 0 {
		return nil, status.Error(codes.InvalidArgument, "assessment_id 不能为空")
	}

	// 调用评估引擎
	err := s.engineService.Evaluate(ctx, req.AssessmentId)
	if err != nil {
		l.Errorw("执行评估失败",
			"action", "evaluate_assessment",
			"assessment_id", req.AssessmentId,
			"result", "failed",
			"error", err.Error(),
		)
		return &pb.EvaluateAssessmentResponse{
			Success: false,
			Status:  "failed",
			Message: err.Error(),
		}, nil
	}

	// 获取评估后的测评信息
	result, err := s.managementService.GetByID(ctx, req.AssessmentId)
	if err != nil {
		l.Warnw("获取评估结果失败",
			"assessment_id", req.AssessmentId,
			"error", err.Error(),
		)
		return &pb.EvaluateAssessmentResponse{
			Success: true,
			Status:  "interpreted",
			Message: "评估完成，但获取结果失败",
		}, nil
	}

	var totalScore float64
	var riskLevel string
	if result.TotalScore != nil {
		totalScore = *result.TotalScore
	}
	if result.RiskLevel != nil {
		riskLevel = *result.RiskLevel
	}

	l.Infow("执行评估成功",
		"action", "evaluate_assessment",
		"assessment_id", req.AssessmentId,
		"total_score", totalScore,
		"risk_level", riskLevel,
		"result", "success",
	)

	return &pb.EvaluateAssessmentResponse{
		Success:    true,
		Status:     "interpreted",
		Message:    "评估完成",
		TotalScore: totalScore,
		RiskLevel:  riskLevel,
	}, nil
}

// TagTestee 给受试者打标签
// 场景：worker 处理 report.generated 事件后调用
// 职责：协议转换，将 gRPC 请求转换为应用服务调用
// 业务逻辑：由 TesteeTaggingService 处理
func (s *InternalService) TagTestee(
	ctx context.Context,
	req *pb.TagTesteeRequest,
) (*pb.TagTesteeResponse, error) {
	l := logger.L(ctx)

	l.Infow("gRPC: 收到给受试者打标签请求",
		"action", "tag_testee",
		"testee_id", req.TesteeId,
		"risk_level", req.RiskLevel,
		"scale_code", req.ScaleCode,
		"high_risk_factors_count", len(req.HighRiskFactors),
		"mark_key_focus", req.MarkKeyFocus,
	)

	// 参数验证
	if req.TesteeId == 0 {
		return nil, status.Error(codes.InvalidArgument, "testee_id 不能为空")
	}

	// 调用应用服务层处理业务逻辑
	// 所有标签更新策略、风险等级判断等业务规则都在应用服务层
	result, err := s.testeeTaggingService.TagByAssessmentResult(
		ctx,
		req.TesteeId,
		req.RiskLevel,
		req.ScaleCode,
		req.HighRiskFactors,
		req.MarkKeyFocus,
	)
	if err != nil {
		l.Errorw("给受试者打标签失败",
			"testee_id", req.TesteeId,
			"risk_level", req.RiskLevel,
			"scale_code", req.ScaleCode,
			"error", err.Error(),
		)
		return nil, status.Errorf(codes.Internal, "给受试者打标签失败: %v", err)
	}

	l.Infow("给受试者打标签成功",
		"action", "tag_testee",
		"testee_id", req.TesteeId,
		"tags_added_count", len(result.TagsAdded),
		"tags_removed_count", len(result.TagsRemoved),
		"key_focus_marked", result.KeyFocusMarked,
	)

	return &pb.TagTesteeResponse{
		Success:        true,
		TagsAdded:      result.TagsAdded,
		KeyFocusMarked: result.KeyFocusMarked,
		Message:        fmt.Sprintf("标签更新成功：添加 %d 个，移除 %d 个", len(result.TagsAdded), len(result.TagsRemoved)),
	}, nil
}

// ==================== Statistics 同步操作 ====================

// SyncDailyStatistics 同步每日统计
// 场景：定时任务调用（推荐使用 REST API: POST /api/v1/statistics/sync/daily）
// 保留此接口作为备用，但推荐使用 Crontab + HTTP 接口方案
func (s *InternalService) SyncDailyStatistics(
	ctx context.Context,
	req *pb.SyncDailyStatisticsRequest,
) (*pb.SyncDailyStatisticsResponse, error) {
	l := logger.L(ctx)

	l.Infow("gRPC: 收到同步每日统计请求",
		"action", "sync_daily_statistics",
	)

	if s.statisticsSyncService == nil {
		return nil, status.Error(codes.Unimplemented, "statistics sync service not available")
	}

	err := s.statisticsSyncService.SyncDailyStatistics(ctx)
	if err != nil {
		l.Errorw("同步每日统计失败",
			"error", err.Error(),
		)
		return &pb.SyncDailyStatisticsResponse{
			Success: false,
			Message: err.Error(),
		}, nil
	}

	l.Infow("同步每日统计成功",
		"action", "sync_daily_statistics",
	)

	return &pb.SyncDailyStatisticsResponse{
		Success: true,
		Message: "同步完成",
	}, nil
}

// SyncAccumulatedStatistics 同步累计统计
// 场景：定时任务调用（推荐使用 REST API: POST /api/v1/statistics/sync/accumulated）
// 保留此接口作为备用，但推荐使用 Crontab + HTTP 接口方案
func (s *InternalService) SyncAccumulatedStatistics(
	ctx context.Context,
	req *pb.SyncAccumulatedStatisticsRequest,
) (*pb.SyncAccumulatedStatisticsResponse, error) {
	l := logger.L(ctx)

	l.Infow("gRPC: 收到同步累计统计请求",
		"action", "sync_accumulated_statistics",
	)

	if s.statisticsSyncService == nil {
		return nil, status.Error(codes.Unimplemented, "statistics sync service not available")
	}

	err := s.statisticsSyncService.SyncAccumulatedStatistics(ctx)
	if err != nil {
		l.Errorw("同步累计统计失败",
			"error", err.Error(),
		)
		return &pb.SyncAccumulatedStatisticsResponse{
			Success: false,
			Message: err.Error(),
		}, nil
	}

	l.Infow("同步累计统计成功",
		"action", "sync_accumulated_statistics",
	)

	return &pb.SyncAccumulatedStatisticsResponse{
		Success: true,
		Message: "同步完成",
	}, nil
}

// SyncPlanStatistics 同步计划统计
// 场景：定时任务调用（推荐使用 REST API: POST /api/v1/statistics/sync/plan）
// 保留此接口作为备用，但推荐使用 Crontab + HTTP 接口方案
func (s *InternalService) SyncPlanStatistics(
	ctx context.Context,
	req *pb.SyncPlanStatisticsRequest,
) (*pb.SyncPlanStatisticsResponse, error) {
	l := logger.L(ctx)

	l.Infow("gRPC: 收到同步计划统计请求",
		"action", "sync_plan_statistics",
	)

	if s.statisticsSyncService == nil {
		return nil, status.Error(codes.Unimplemented, "statistics sync service not available")
	}

	err := s.statisticsSyncService.SyncPlanStatistics(ctx)
	if err != nil {
		l.Errorw("同步计划统计失败",
			"error", err.Error(),
		)
		return &pb.SyncPlanStatisticsResponse{
			Success: false,
			Message: err.Error(),
		}, nil
	}

	l.Infow("同步计划统计成功",
		"action", "sync_plan_statistics",
	)

	return &pb.SyncPlanStatisticsResponse{
		Success: true,
		Message: "同步完成",
	}, nil
}

// ValidateStatistics 校验统计数据一致性
// 场景：定时任务调用（推荐使用 REST API: POST /api/v1/statistics/validate）
// 保留此接口作为备用，但推荐使用 Crontab + HTTP 接口方案
func (s *InternalService) ValidateStatistics(
	ctx context.Context,
	req *pb.ValidateStatisticsRequest,
) (*pb.ValidateStatisticsResponse, error) {
	l := logger.L(ctx)

	l.Infow("gRPC: 收到校验统计数据一致性请求",
		"action", "validate_statistics",
	)

	if s.statisticsValidatorService == nil {
		return nil, status.Error(codes.Unimplemented, "statistics validator service not available")
	}

	err := s.statisticsValidatorService.ValidateConsistency(ctx)
	if err != nil {
		l.Errorw("校验统计数据一致性失败",
			"error", err.Error(),
		)
		return &pb.ValidateStatisticsResponse{
			Success:    false,
			Consistent: false,
			Message:    err.Error(),
		}, nil
	}

	l.Infow("校验统计数据一致性成功",
		"action", "validate_statistics",
	)

	return &pb.ValidateStatisticsResponse{
		Success:    true,
		Consistent: true,
		Message:    "数据一致",
	}, nil
}

// ==================== Plan 调度操作 ====================

// SchedulePendingTasks 调度待推送任务
// 场景：定时任务调用（推荐使用 REST API: POST /api/v1/plans/tasks/schedule）
// 保留此接口作为备用，但推荐使用 Crontab + HTTP 接口方案
func (s *InternalService) SchedulePendingTasks(
	ctx context.Context,
	req *pb.SchedulePendingTasksRequest,
) (*pb.SchedulePendingTasksResponse, error) {
	l := logger.L(ctx)

	l.Infow("gRPC: 收到调度待推送任务请求",
		"action", "schedule_pending_tasks",
		"before", req.Before,
	)

	if s.taskSchedulerService == nil {
		return nil, status.Error(codes.Unimplemented, "task scheduler service not available")
	}

	before := req.Before
	if before == "" {
		before = "" // 使用默认值（当前时间）
	}

	tasks, err := s.taskSchedulerService.SchedulePendingTasks(ctx, before)
	if err != nil {
		l.Errorw("调度待推送任务失败",
			"error", err.Error(),
		)
		return &pb.SchedulePendingTasksResponse{
			Success: false,
			Message: err.Error(),
		}, nil
	}

	l.Infow("调度待推送任务成功",
		"action", "schedule_pending_tasks",
		"scheduled_count", len(tasks),
	)

	return &pb.SchedulePendingTasksResponse{
		Success:        true,
		ScheduledCount: int64(len(tasks)),
		Message:        fmt.Sprintf("成功调度 %d 个任务", len(tasks)),
	}, nil
}

// ==================== 小程序码生成操作 ====================

// GenerateQuestionnaireQRCode 生成问卷小程序码
// 场景：worker 处理 questionnaire.published 事件后调用
func (s *InternalService) GenerateQuestionnaireQRCode(
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
// 场景：worker 处理 scale.published 事件后调用
func (s *InternalService) GenerateScaleQRCode(
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
