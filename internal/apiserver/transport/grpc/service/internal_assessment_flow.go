package service

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/FangcunMount/component-base/pkg/logger"
	pb "github.com/FangcunMount/qs-server/api/grpc/gen/internalapi"
	assessmentApp "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/assessment"
	statisticsApp "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	assessmentDomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/reportstatus"
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
	if req == nil || req.EventId == "" || req.EventType == "" || req.OccurredAt == nil {
		return nil, status.Error(codes.InvalidArgument, "event_id, event_type, org_id and occurred_at are required")
	}

	orgID, err := requestPlanOrgID(ctx, req.OrgId)
	if err != nil {
		return nil, err
	}

	result, err := s.behaviorProjectorService.ProjectBehaviorEvent(ctx, statisticsApp.BehaviorProjectEventInput{
		EventID:           req.EventId,
		EventType:         req.EventType,
		OrgID:             orgID,
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

	if s.answerSheetScoringService != nil {
		if err := s.answerSheetScoringService.CalculateAndSave(ctx, req.AnswersheetId); err != nil {
			l.Errorw("答卷计分失败",
				"answersheet_id", req.AnswersheetId,
				"error", err.Error(),
			)
			return nil, status.Errorf(codes.Internal, "答卷计分失败: %v", err)
		}
	}

	orgID, err := requestOrgIDUint64(ctx, req.OrgId)
	if err != nil {
		return nil, err
	}
	req.OrgId = orgID

	dto, err := buildCreateAssessmentDTO(ctx, req, s.assessmentBindingResolver)
	if err != nil {
		l.Errorw("解析解释模型绑定失败",
			"action", "create_assessment_from_answersheet",
			"questionnaire_code", questionnaireCode,
			"questionnaire_version", req.QuestionnaireVersion,
			"error", err,
		)
		return nil, status.Errorf(codes.Internal, "解析解释模型绑定失败: %v", err)
	}
	matchedTask := s.applyMatchedTaskOrigin(ctx, l, req, dto.ModelCode, &dto)

	if response, ok := s.loadExistingAssessmentResponse(ctx, l, req.AnswersheetId, req.OrgId, matchedTask); ok {
		return response, nil
	}

	return s.createAssessmentFromAnswerSheet(ctx, l, req, dto, matchedTask, shouldAutoSubmitAssessment(dto))
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

	err := s.executeService.Evaluate(ctx, req.AssessmentId)
	if err != nil {
		l.Errorw("执行评估失败",
			"action", "evaluate_assessment",
			"assessment_id", req.AssessmentId,
			"result", "failed",
			"error", err.Error(),
		)
		return evaluateFailureResponse(ctx, s.runQueryService, req.AssessmentId, err.Error()), nil
	}

	result, err := s.managementService.GetByID(ctx, req.AssessmentId)
	if err != nil {
		l.Warnw("获取评估结果失败",
			"assessment_id", req.AssessmentId,
			"error", err.Error(),
		)
		resp := &pb.EvaluateAssessmentResponse{
			Success: true,
			Status:  "evaluated",
			Message: "评估完成，但获取结果失败",
		}
		applyLatestRunAuditMetadata(ctx, s.runQueryService, req.AssessmentId, func(traceID, inputSnapshotRef string) {
			resp.TraceId = traceID
			resp.InputSnapshotRef = inputSnapshotRef
		})
		return resp, nil
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
		"outcome", outcomeSummaryFromAssessmentResult(result),
		"total_score", totalScore,
		"risk_level", riskLevel,
		"result", "success",
	)

	resp := &pb.EvaluateAssessmentResponse{
		Success: true,
		Status:  assessmentResultStatus(result),
		Message: "评估完成",
		Outcome: outcomeSummaryFromAssessmentResult(result),
	}
	applyLatestRunAuditMetadata(ctx, s.runQueryService, req.AssessmentId, func(traceID, inputSnapshotRef string) {
		resp.TraceId = traceID
		resp.InputSnapshotRef = inputSnapshotRef
	})
	return resp, nil
}

func (flow assessmentFlow) GenerateReportFromAssessment(
	ctx context.Context,
	req *pb.GenerateReportFromAssessmentRequest,
) (*pb.GenerateReportFromAssessmentResponse, error) {
	s := flow.service
	l := logger.L(ctx)

	if req == nil || (req.AssessmentId == 0 && req.OutcomeId == "") {
		return nil, status.Error(codes.InvalidArgument, "assessment_id 或 outcome_id 不能为空")
	}

	l.Infow("gRPC: 收到生成报告请求",
		"action", "generate_report_from_assessment",
		"assessment_id", req.AssessmentId,
		"outcome_id", req.OutcomeId,
	)

	if s.outcomeReportService == nil {
		return generateReportFailureResponse(ctx, s.runQueryService, req.AssessmentId, "interpretation outcome service is not configured"), nil
	}
	var rpt *domainreport.InterpretReport
	var err error
	if req.OutcomeId != "" {
		outcomeID, parseErr := meta.ParseID(req.OutcomeId)
		if parseErr != nil || outcomeID.IsZero() {
			return nil, status.Error(codes.InvalidArgument, "outcome_id 无效")
		}
		rpt, err = s.outcomeReportService.GenerateByOutcomeID(ctx, outcomeID)
	} else {
		rpt, err = s.outcomeReportService.GenerateByAssessmentID(ctx, meta.FromUint64(req.AssessmentId))
	}
	if err != nil {
		l.Errorw("生成报告失败",
			"assessment_id", req.AssessmentId,
			"error", err.Error(),
		)
		return generateReportFailureResponse(ctx, s.runQueryService, req.AssessmentId, err.Error()), nil
	}
	if s.reportStatusReporter != nil && rpt != nil {
		id := reportstatus.AssessmentKey(rpt.ID().Uint64())
		s.reportStatusReporter.SetCompleted(ctx, id, "", id)
	}

	return &pb.GenerateReportFromAssessmentResponse{
		Success: true,
		Status:  "generated",
		Message: "报告生成完成",
	}, nil
}

func assessmentResultStatus(result *assessmentApp.AssessmentResult) string {
	if result == nil {
		return "evaluated"
	}
	switch result.Status {
	case string(assessmentDomain.StatusEvaluated):
		return "evaluated"
	case string(assessmentDomain.StatusFailed):
		return "failed"
	default:
		return result.Status
	}
}

func (flow assessmentFlow) SyncAssessmentAttention(
	ctx context.Context,
	req *pb.SyncAssessmentAttentionRequest,
) (*pb.SyncAssessmentAttentionResponse, error) {
	s := flow.service
	l := logger.L(ctx)

	l.Infow("gRPC: 收到同步测评后置关注请求",
		"action", "sync_assessment_attention",
		"testee_id", req.TesteeId,
		"risk_level", req.RiskLevel,
		"mark_key_focus", req.MarkKeyFocus,
	)

	if req.TesteeId == 0 {
		return nil, status.Error(codes.InvalidArgument, "testee_id 不能为空")
	}

	result, err := s.assessmentAttentionService.SyncAssessmentAttention(
		ctx,
		req.TesteeId,
		req.RiskLevel,
		req.MarkKeyFocus,
	)
	if err != nil {
		l.Errorw("同步测评后置关注失败",
			"testee_id", req.TesteeId,
			"risk_level", req.RiskLevel,
			"error", err.Error(),
		)
		return nil, status.Errorf(codes.Internal, "同步测评后置关注失败: %v", err)
	}

	l.Infow("同步测评后置关注成功",
		"action", "sync_assessment_attention",
		"testee_id", req.TesteeId,
		"key_focus_marked", result.KeyFocusMarked,
	)

	return &pb.SyncAssessmentAttentionResponse{
		Success:        true,
		KeyFocusMarked: result.KeyFocusMarked,
		Message:        "测评后置关注同步完成",
	}, nil
}
