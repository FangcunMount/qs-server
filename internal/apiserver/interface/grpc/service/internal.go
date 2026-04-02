package service

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	operatorApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/operator"
	testeeApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/testee"
	assessmentApp "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/engine"
	qrcodeApp "github.com/FangcunMount/qs-server/internal/apiserver/application/qrcode"
	answerSheetApp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/answersheet"
	domainoperator "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/operator"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	iaminfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	pb "github.com/FangcunMount/qs-server/internal/apiserver/interface/grpc/proto/internalapi"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
)

// InternalService 内部 gRPC 服务 - 供 Worker 调用
// 用于事件处理后的业务逻辑调用。
type InternalService struct {
	pb.UnimplementedInternalServiceServer
	answerSheetScoringService answerSheetApp.AnswerSheetScoringService
	submissionService         assessmentApp.AssessmentSubmissionService
	managementService         assessmentApp.AssessmentManagementService
	engineService             engine.Service
	scaleRepo                 scale.Repository
	testeeTaggingService      testeeApp.TesteeTaggingService
	operatorLifecycleService  operatorApp.OperatorLifecycleService
	operatorAuthService       operatorApp.OperatorAuthorizationService
	operatorQueryService      operatorApp.OperatorQueryService
	operatorRepo              domainoperator.Repository
	authzSnapshot             *iaminfra.AuthzSnapshotLoader
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
	operatorLifecycleService operatorApp.OperatorLifecycleService,
	operatorAuthService operatorApp.OperatorAuthorizationService,
	operatorQueryService operatorApp.OperatorQueryService,
	operatorRepo domainoperator.Repository,
	authzSnapshot *iaminfra.AuthzSnapshotLoader,
	qrCodeService interface{}, // qrcodeApp.QRCodeService，可能为 nil
) *InternalService {
	var qrService qrcodeApp.QRCodeService
	if q, ok := qrCodeService.(qrcodeApp.QRCodeService); ok {
		qrService = q
	}

	return &InternalService{
		answerSheetScoringService: answerSheetScoringService,
		submissionService:         submissionService,
		managementService:         managementService,
		engineService:             engineService,
		scaleRepo:                 scaleRepo,
		testeeTaggingService:      testeeTaggingService,
		operatorLifecycleService:  operatorLifecycleService,
		operatorAuthService:       operatorAuthService,
		operatorQueryService:      operatorQueryService,
		operatorRepo:              operatorRepo,
		authzSnapshot:             authzSnapshot,
		qrCodeService:             qrService,
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

	// 幂等：先查是否已存在
	if existing, err := s.submissionService.GetMyAssessmentByAnswerSheetID(ctx, req.AnswersheetId); err == nil && existing != nil {
		l.Infow("检测到答卷已创建测评，直接返回",
			"answersheet_id", req.AnswersheetId,
			"assessment_id", existing.ID,
		)
		return &pb.CreateAssessmentFromAnswerSheetResponse{
			AssessmentId:  existing.ID,
			Created:       false,
			AutoSubmitted: false,
			Message:       "测评已存在",
		}, nil
	}

	// 调用应用服务创建测评
	result, err := s.submissionService.Create(ctx, dto)
	if err != nil {
		// 如果是唯一约束冲突，查出已有测评并返回
		if errors.IsCode(err, errorCode.ErrAssessmentDuplicate) {
			if existing, findErr := s.submissionService.GetMyAssessmentByAnswerSheetID(ctx, req.AnswersheetId); findErr == nil && existing != nil {
				l.Infow("测评已存在，返回已有结果",
					"answersheet_id", req.AnswersheetId,
					"assessment_id", existing.ID,
				)
				return &pb.CreateAssessmentFromAnswerSheetResponse{
					AssessmentId:  existing.ID,
					Created:       false,
					AutoSubmitted: false,
					Message:       "测评已存在",
				}, nil
			}
		}

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

// BootstrapOperator 自举首个操作者。
func (s *InternalService) BootstrapOperator(
	ctx context.Context,
	req *pb.BootstrapOperatorRequest,
) (*pb.BootstrapOperatorResponse, error) {
	l := logger.L(ctx)
	l.Infow("gRPC: 收到 operator bootstrap 请求",
		"action", "bootstrap_operator",
		"org_id", req.OrgId,
		"user_id", req.UserId,
	)

	if s.operatorLifecycleService == nil || s.operatorQueryService == nil {
		return nil, status.Error(codes.FailedPrecondition, "operator services not configured")
	}
	if req.OrgId <= 0 {
		return nil, status.Error(codes.InvalidArgument, "org_id 不能为空")
	}
	if req.UserId <= 0 {
		return nil, status.Error(codes.InvalidArgument, "user_id 不能为空")
	}
	if req.Name == "" {
		return nil, status.Error(codes.InvalidArgument, "name 不能为空")
	}

	created := false
	if _, err := s.operatorQueryService.GetByUser(ctx, req.OrgId, req.UserId); err != nil {
		if errors.IsCode(err, errorCode.ErrUserNotFound) {
			created = true
		} else {
			return nil, status.Errorf(codes.Internal, "query existing operator failed: %v", err)
		}
	}

	result, err := s.operatorLifecycleService.EnsureByUser(ctx, req.OrgId, req.UserId, req.Name)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "ensure operator failed: %v", err)
	}

	if req.Name != "" || req.Email != "" || req.Phone != "" {
		if err := s.operatorLifecycleService.UpdateFromExternalSource(ctx, result.ID, req.Name, req.Email, req.Phone); err != nil {
			return nil, status.Errorf(codes.Internal, "sync operator profile failed: %v", err)
		}
	}

	if s.operatorAuthService != nil {
		if req.IsActive {
			if err := s.operatorAuthService.Activate(ctx, result.ID); err != nil {
				return nil, status.Errorf(codes.Internal, "activate operator failed: %v", err)
			}
		} else {
			if err := s.operatorAuthService.Deactivate(ctx, result.ID); err != nil {
				return nil, status.Errorf(codes.Internal, "deactivate operator failed: %v", err)
			}
		}
	}

	if req.IsActive && s.authzSnapshot != nil && s.operatorRepo != nil {
		op, err := s.operatorRepo.FindByID(ctx, domainoperator.ID(result.ID))
		if err != nil {
			return nil, status.Errorf(codes.Internal, "load operator aggregate failed: %v", err)
		}
		if _, err := iaminfra.SyncAndPersistOperatorRolesFromSnapshot(ctx, s.authzSnapshot, s.operatorRepo, req.OrgId, op); err != nil {
			return nil, status.Errorf(codes.Internal, "sync operator roles from snapshot failed: %v", err)
		}
	}

	finalResult, err := s.operatorQueryService.GetByUser(ctx, req.OrgId, req.UserId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "query operator after bootstrap failed: %v", err)
	}

	message := "operator already exists"
	if created {
		message = "operator bootstrapped"
	}
	l.Infow("operator bootstrap 完成",
		"action", "bootstrap_operator",
		"org_id", req.OrgId,
		"user_id", req.UserId,
		"operator_id", finalResult.ID,
		"created", created,
		"roles", finalResult.Roles,
	)

	return &pb.BootstrapOperatorResponse{
		OperatorId: finalResult.ID,
		Created:    created,
		Message:    message,
		Roles:      append([]string(nil), finalResult.Roles...),
	}, nil
}
