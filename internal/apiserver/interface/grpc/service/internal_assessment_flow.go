package service

import (
	"context"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/FangcunMount/component-base/pkg/logger"
	assessmentApp "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/assessment"
	statisticsApp "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	planDomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	pb "github.com/FangcunMount/qs-server/internal/apiserver/interface/grpc/proto/internalapi"
)

type behaviorProjectionFlow struct {
	service *InternalService
}

type assessmentFlow struct {
	service *InternalService
}

func newBehaviorProjectionFlow(service *InternalService) behaviorProjectionFlow {
	return behaviorProjectionFlow{service: service}
}

func newAssessmentFlow(service *InternalService) assessmentFlow {
	return assessmentFlow{service: service}
}

func (flow behaviorProjectionFlow) ProjectBehaviorEvent(
	ctx context.Context,
	req *pb.ProjectBehaviorEventRequest,
) (*pb.ProjectBehaviorEventResponse, error) {
	s := flow.service
	if s.behaviorProjectorService == nil {
		return nil, status.Error(codes.FailedPrecondition, "behavior projector service is not available")
	}
	if req == nil || req.EventId == "" || req.EventType == "" || req.OrgId == 0 || req.OccurredAt == nil {
		return nil, status.Error(codes.InvalidArgument, "event_id, event_type, org_id and occurred_at are required")
	}

	result, err := s.behaviorProjectorService.ProjectBehaviorEvent(ctx, statisticsApp.BehaviorProjectEventInput{
		EventID:           req.EventId,
		EventType:         req.EventType,
		OrgID:             req.OrgId,
		ClinicianID:       req.ClinicianId,
		SourceClinicianID: req.SourceClinicianId,
		EntryID:           req.EntryId,
		TesteeID:          req.TesteeId,
		AnswerSheetID:     req.AnswersheetId,
		AssessmentID:      req.AssessmentId,
		ReportID:          req.ReportId,
		FailureReason:     req.FailureReason,
		OccurredAt:        req.OccurredAt.AsTime(),
	})
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &pb.ProjectBehaviorEventResponse{
		Status:  string(result.Status),
		Message: "ok",
	}, nil
}

func (flow assessmentFlow) CalculateAnswerSheetScore(
	ctx context.Context,
	req *pb.CalculateAnswerSheetScoreRequest,
) (*pb.CalculateAnswerSheetScoreResponse, error) {
	s := flow.service
	l := logger.L(ctx)
	var answerSheetID uint64
	if req != nil {
		answerSheetID = req.AnswersheetId
	}

	l.Infow("gRPC: 收到答卷计分请求",
		"action", "calculate_answersheet_score",
		"answersheet_id", answerSheetID,
	)

	if req == nil || req.AnswersheetId == 0 {
		return &pb.CalculateAnswerSheetScoreResponse{
			Success: false,
			Message: "answersheet_id 不能为空",
		}, nil
	}

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

func (flow assessmentFlow) CreateAssessmentFromAnswerSheet(
	ctx context.Context,
	req *pb.CreateAssessmentFromAnswerSheetRequest,
) (*pb.CreateAssessmentFromAnswerSheetResponse, error) {
	s := flow.service
	l := logger.L(ctx)
	var (
		answerSheetID     uint64
		questionnaireCode string
		fillerID          uint64
		taskID            string
	)
	if req != nil {
		answerSheetID = req.AnswersheetId
		questionnaireCode = req.QuestionnaireCode
		fillerID = req.FillerId
		taskID = req.TaskId
	}

	l.Infow("gRPC: 收到从答卷创建测评请求",
		"action", "create_assessment_from_answersheet",
		"answersheet_id", answerSheetID,
		"questionnaire_code", questionnaireCode,
		"filler_id", fillerID,
		"task_id", taskID,
	)

	if err := validateCreateAssessmentFromAnswerSheetRequest(req); err != nil {
		return nil, err
	}

	scaleCtx := s.resolveAssessmentScaleContext(ctx, req.QuestionnaireCode)
	dto := buildCreateAssessmentDTO(req, scaleCtx)
	matchedTask := s.applyMatchedTaskOrigin(ctx, l, req, scaleCtx.medicalScaleCode, &dto)

	if response, ok := s.loadExistingAssessmentResponse(ctx, l, req.AnswersheetId, req.OrgId, matchedTask); ok {
		return response, nil
	}

	return s.createAssessmentFromAnswerSheet(ctx, l, req, dto, matchedTask, scaleCtx.medicalScaleID != nil)
}

func (flow assessmentFlow) EvaluateAssessment(
	ctx context.Context,
	req *pb.EvaluateAssessmentRequest,
) (*pb.EvaluateAssessmentResponse, error) {
	s := flow.service
	l := logger.L(ctx)

	l.Infow("gRPC: 收到执行评估请求",
		"action", "evaluate_assessment",
		"assessment_id", req.AssessmentId,
	)

	if req.AssessmentId == 0 {
		return nil, status.Error(codes.InvalidArgument, "assessment_id 不能为空")
	}

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

func (flow assessmentFlow) TagTestee(
	ctx context.Context,
	req *pb.TagTesteeRequest,
) (*pb.TagTesteeResponse, error) {
	s := flow.service
	l := logger.L(ctx)

	l.Infow("gRPC: 收到给受试者打标签请求",
		"action", "tag_testee",
		"testee_id", req.TesteeId,
		"risk_level", req.RiskLevel,
		"scale_code", req.ScaleCode,
		"high_risk_factors_count", len(req.HighRiskFactors),
		"mark_key_focus", req.MarkKeyFocus,
	)

	if req.TesteeId == 0 {
		return nil, status.Error(codes.InvalidArgument, "testee_id 不能为空")
	}

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

func (flow assessmentFlow) applyMatchedTaskOrigin(
	ctx context.Context,
	req *pb.CreateAssessmentFromAnswerSheetRequest,
	medicalScaleCode *string,
	dto *assessmentApp.CreateAssessmentDTO,
) *planDomain.AssessmentTask {
	l := logger.L(ctx)
	return flow.service.applyMatchedTaskOrigin(ctx, l, req, medicalScaleCode, dto)
}
