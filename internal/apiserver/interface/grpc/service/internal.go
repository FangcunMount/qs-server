package service

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/FangcunMount/component-base/pkg/logger"
	assessmentApp "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/engine"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	pb "github.com/FangcunMount/qs-server/internal/apiserver/interface/grpc/proto/internalapi"
)

// InternalService 内部 gRPC 服务 - 供 Worker 调用
// 用于事件处理后的业务逻辑调用
type InternalService struct {
	pb.UnimplementedInternalServiceServer
	submissionService assessmentApp.AssessmentSubmissionService
	managementService assessmentApp.AssessmentManagementService
	engineService     engine.Service
	scaleRepo         scale.Repository
	testeeRepo        testee.Repository
}

// NewInternalService 创建内部 gRPC 服务
func NewInternalService(
	submissionService assessmentApp.AssessmentSubmissionService,
	managementService assessmentApp.AssessmentManagementService,
	engineService engine.Service,
	scaleRepo scale.Repository,
	testeeRepo testee.Repository,
) *InternalService {
	return &InternalService{
		submissionService: submissionService,
		managementService: managementService,
		engineService:     engineService,
		scaleRepo:         scaleRepo,
		testeeRepo:        testeeRepo,
	}
}

// RegisterService 注册 gRPC 服务
func (s *InternalService) RegisterService(server *grpc.Server) {
	pb.RegisterInternalServiceServer(server, s)
}

// CreateAssessmentFromAnswerSheet 从答卷创建测评
// 场景：worker 处理 answersheet.submitted 事件后调用
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

	// 确定受试者ID（当 filler_type 为 self 时，filler_id 即为 testee_id）
	var testeeID uint64
	if req.FillerType == "self" {
		testeeID = req.FillerId
	} else {
		// 代填场景：当前系统设计中答卷未存储受试者ID，暂时使用 filler_id
		// TODO: 在 proto 中添加 testee_id 字段以支持代填场景
		testeeID = req.FillerId
		l.Warnw("代填场景使用 filler_id 作为 testee_id",
			"filler_type", req.FillerType,
			"filler_id", req.FillerId,
		)
	}

	// 查询受试者获取 OrgID
	testeeEntity, err := s.testeeRepo.FindByID(ctx, testee.ID(testeeID))
	if err != nil {
		l.Errorw("查询受试者失败",
			"testee_id", testeeID,
			"error", err.Error(),
		)
		return nil, status.Errorf(codes.NotFound, "受试者不存在: %v", err)
	}

	// 构建创建 DTO（使用 QuestionnaireCode 作为唯一标识）
	dto := assessmentApp.CreateAssessmentDTO{
		OrgID:                uint64(testeeEntity.OrgID()),
		TesteeID:             testeeID,
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
