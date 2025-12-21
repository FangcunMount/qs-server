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
	answerSheetApp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/answersheet"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	pb "github.com/FangcunMount/qs-server/internal/apiserver/interface/grpc/proto/internalapi"
)

// InternalService 内部 gRPC 服务 - 供 Worker 调用
// 用于事件处理后的业务逻辑调用
type InternalService struct {
	pb.UnimplementedInternalServiceServer
	answerSheetScoringService answerSheetApp.AnswerSheetScoringService
	submissionService         assessmentApp.AssessmentSubmissionService
	managementService         assessmentApp.AssessmentManagementService
	engineService             engine.Service
	scaleRepo                 scale.Repository
	testeeTaggingService      testeeApp.TesteeTaggingService
}

// NewInternalService 创建内部 gRPC 服务
func NewInternalService(
	answerSheetScoringService answerSheetApp.AnswerSheetScoringService,
	submissionService assessmentApp.AssessmentSubmissionService,
	managementService assessmentApp.AssessmentManagementService,
	engineService engine.Service,
	scaleRepo scale.Repository,
	testeeTaggingService testeeApp.TesteeTaggingService,
) *InternalService {
	return &InternalService{
		answerSheetScoringService: answerSheetScoringService,
		submissionService:         submissionService,
		managementService:         managementService,
		engineService:             engineService,
		scaleRepo:                 scaleRepo,
		testeeTaggingService:      testeeTaggingService,
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
