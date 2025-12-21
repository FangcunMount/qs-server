package evaluation

import (
	"context"
	"strconv"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/collection-server/infra/grpcclient"
)

// QueryService 测评查询服务
// 作为 BFF 层的薄服务，主要职责：
// 1. 调用 apiserver 的 gRPC 服务
// 2. 转换 gRPC 响应到 REST DTO
type QueryService struct {
	evaluationClient *grpcclient.EvaluationClient
}

// NewQueryService 创建测评查询服务
func NewQueryService(
	evaluationClient *grpcclient.EvaluationClient,
) *QueryService {
	return &QueryService{
		evaluationClient: evaluationClient,
	}
}

// GetMyAssessment 获取我的测评详情
func (s *QueryService) GetMyAssessment(ctx context.Context, testeeID, assessmentID uint64) (*AssessmentDetailResponse, error) {
	l := logger.L(ctx)
	startTime := time.Now()

	log.Infof("Getting my assessment: testeeID=%d, assessmentID=%d", testeeID, assessmentID)

	l.Debugw("获取我的测评详情",
		"action", "get_my_assessment",
		"testee_id", testeeID,
		"assessment_id", assessmentID,
	)

	result, err := s.evaluationClient.GetMyAssessment(ctx, testeeID, assessmentID)
	if err != nil {
		log.Errorf("Failed to get assessment via gRPC: %v", err)
		l.Errorw("获取测评失败",
			"action", "get_my_assessment",
			"testee_id", testeeID,
			"assessment_id", assessmentID,
			"result", "failed",
			"error", err.Error(),
		)
		return nil, err
	}

	if result == nil {
		l.Warnw("获取的测评为空",
			"assessment_id", assessmentID,
		)
		return nil, nil
	}

	duration := time.Since(startTime)
	l.Debugw("获取测评成功",
		"assessment_id", assessmentID,
		"status", result.Status,
		"duration_ms", duration.Milliseconds(),
	)

	// 转换 AnswerSheetID，如果为 0 则转换为空字符串
	answerSheetID := ""
	if result.AnswerSheetID != 0 {
		answerSheetID = strconv.FormatUint(result.AnswerSheetID, 10)
	}

	return &AssessmentDetailResponse{
		ID:                   strconv.FormatUint(result.ID, 10),
		OrgID:                strconv.FormatUint(result.OrgID, 10),
		TesteeID:             strconv.FormatUint(result.TesteeID, 10),
		QuestionnaireCode:    result.QuestionnaireCode,
		QuestionnaireVersion: result.QuestionnaireVersion,
		AnswerSheetID:        answerSheetID,
		ScaleCode:            result.ScaleCode,
		ScaleName:            result.ScaleName,
		OriginType:           result.OriginType,
		OriginID:             result.OriginID,
		Status:               result.Status,
		TotalScore:           result.TotalScore,
		RiskLevel:            result.RiskLevel,
		CreatedAt:            result.CreatedAt,
		SubmittedAt:          result.SubmittedAt,
		InterpretedAt:        result.InterpretedAt,
		FailedAt:             result.FailedAt,
		FailureReason:        result.FailureReason,
	}, nil
}

// GetMyAssessmentByAnswerSheetID 通过答卷ID获取测评详情
func (s *QueryService) GetMyAssessmentByAnswerSheetID(ctx context.Context, answerSheetID uint64) (*AssessmentDetailResponse, error) {
	l := logger.L(ctx)
	startTime := time.Now()

	log.Infof("Getting assessment by answer sheet: answerSheetID=%d", answerSheetID)

	l.Debugw("通过答卷ID获取测评详情",
		"action", "get_assessment_by_answersheet",
		"answer_sheet_id", answerSheetID,
	)

	result, err := s.evaluationClient.GetMyAssessmentByAnswerSheetID(ctx, answerSheetID)
	if err != nil {
		log.Errorf("Failed to get assessment by answer sheet via gRPC: %v", err)
		l.Errorw("通过答卷ID获取测评失败",
			"action", "get_assessment_by_answersheet",
			"answer_sheet_id", answerSheetID,
			"result", "failed",
			"error", err.Error(),
		)
		return nil, err
	}

	if result == nil {
		l.Warnw("获取的测评为空",
			"answer_sheet_id", answerSheetID,
		)
		return nil, nil
	}

	duration := time.Since(startTime)
	l.Debugw("通过答卷ID获取测评成功",
		"answer_sheet_id", answerSheetID,
		"assessment_id", result.ID,
		"status", result.Status,
		"duration_ms", duration.Milliseconds(),
	)

	// 转换 AnswerSheetID，如果为 0 则转换为空字符串
	answerSheetIDStr := ""
	if result.AnswerSheetID != 0 {
		answerSheetIDStr = strconv.FormatUint(result.AnswerSheetID, 10)
	}

	return &AssessmentDetailResponse{
		ID:                   strconv.FormatUint(result.ID, 10),
		OrgID:                strconv.FormatUint(result.OrgID, 10),
		TesteeID:             strconv.FormatUint(result.TesteeID, 10),
		QuestionnaireCode:    result.QuestionnaireCode,
		QuestionnaireVersion: result.QuestionnaireVersion,
		AnswerSheetID:        answerSheetIDStr,
		ScaleCode:            result.ScaleCode,
		ScaleName:            result.ScaleName,
		OriginType:           result.OriginType,
		OriginID:             result.OriginID,
		Status:               result.Status,
		TotalScore:           result.TotalScore,
		RiskLevel:            result.RiskLevel,
		CreatedAt:            result.CreatedAt,
		SubmittedAt:          result.SubmittedAt,
		InterpretedAt:        result.InterpretedAt,
		FailedAt:             result.FailedAt,
		FailureReason:        result.FailureReason,
	}, nil
}

// ListMyAssessments 获取我的测评列表
func (s *QueryService) ListMyAssessments(ctx context.Context, testeeID uint64, req *ListAssessmentsRequest) (*ListAssessmentsResponse, error) {
	l := logger.L(ctx)
	startTime := time.Now()

	log.Infof("Listing my assessments: testeeID=%d, page=%d, pageSize=%d", testeeID, req.Page, req.PageSize)

	l.Debugw("查询我的测评列表",
		"action", "list_my_assessments",
		"testee_id", testeeID,
		"page", req.Page,
		"page_size", req.PageSize,
		"status_filter", req.Status,
	)

	// 默认分页参数
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = 50
	}
	// 最大分页限制，避免一次查询过多数据
	if req.PageSize > 100 {
		req.PageSize = 100
	}

	l.Debugw("开始从 gRPC 服务查询测评列表",
		"testee_id", testeeID,
		"page", req.Page,
		"page_size", req.PageSize,
	)

	result, err := s.evaluationClient.ListMyAssessments(ctx, testeeID, req.Status, req.Page, req.PageSize)
	if err != nil {
		log.Errorf("Failed to list assessments via gRPC: %v", err)
		l.Errorw("查询测评列表失败",
			"action", "list_my_assessments",
			"testee_id", testeeID,
			"result", "failed",
			"error", err.Error(),
		)
		return nil, err
	}

	items := make([]AssessmentSummaryResponse, len(result.Items))
	for i, item := range result.Items {
		// 转换 AnswerSheetID，如果为 0 则转换为空字符串
		answerSheetID := ""
		if item.AnswerSheetID != 0 {
			answerSheetID = strconv.FormatUint(item.AnswerSheetID, 10)
		}

		items[i] = AssessmentSummaryResponse{
			ID:                   strconv.FormatUint(item.ID, 10),
			QuestionnaireCode:    item.QuestionnaireCode,
			QuestionnaireVersion: item.QuestionnaireVersion,
			AnswerSheetID:        answerSheetID,
			ScaleCode:            item.ScaleCode,
			ScaleName:            item.ScaleName,
			OriginType:           item.OriginType,
			Status:               item.Status,
			TotalScore:           item.TotalScore,
			RiskLevel:            item.RiskLevel,
			CreatedAt:            item.CreatedAt,
			SubmittedAt:          item.SubmittedAt,
			InterpretedAt:        item.InterpretedAt,
		}
	}

	duration := time.Since(startTime)
	l.Debugw("查询我的测评列表成功",
		"action", "list_my_assessments",
		"result", "success",
		"testee_id", testeeID,
		"total_count", result.Total,
		"page_count", len(items),
		"duration_ms", duration.Milliseconds(),
	)

	return &ListAssessmentsResponse{
		Items:      items,
		Total:      result.Total,
		Page:       result.Page,
		PageSize:   result.PageSize,
		TotalPages: result.TotalPages,
	}, nil
}

// GetAssessmentScores 获取测评得分详情
func (s *QueryService) GetAssessmentScores(ctx context.Context, testeeID, assessmentID uint64) ([]FactorScoreResponse, error) {
	l := logger.L(ctx)
	startTime := time.Now()

	log.Infof("Getting assessment scores: testeeID=%d, assessmentID=%d", testeeID, assessmentID)

	l.Debugw("获取测评得分详情",
		"action", "get_assessment_scores",
		"testee_id", testeeID,
		"assessment_id", assessmentID,
	)

	result, err := s.evaluationClient.GetAssessmentScores(ctx, testeeID, assessmentID)
	if err != nil {
		log.Errorf("Failed to get assessment scores via gRPC: %v", err)
		l.Errorw("获取测评得分失败",
			"action", "get_assessment_scores",
			"assessment_id", assessmentID,
			"result", "failed",
			"error", err.Error(),
		)
		return nil, err
	}

	duration := time.Since(startTime)
	l.Debugw("获取测评得分成功",
		"action", "get_assessment_scores",
		"assessment_id", assessmentID,
		"score_count", len(result),
		"duration_ms", duration.Milliseconds(),
	)

	scores := make([]FactorScoreResponse, len(result))
	for i, score := range result {
		scores[i] = FactorScoreResponse{
			FactorCode:   score.FactorCode,
			FactorName:   score.FactorName,
			RawScore:     score.RawScore,
			RiskLevel:    score.RiskLevel,
			Conclusion:   score.Conclusion,
			Suggestion:   score.Suggestion,
			IsTotalScore: score.IsTotalScore,
		}
	}

	return scores, nil
}

// GetAssessmentReport 获取测评报告
func (s *QueryService) GetAssessmentReport(ctx context.Context, assessmentID uint64) (*AssessmentReportResponse, error) {
	log.Infof("Getting assessment report: assessmentID=%d", assessmentID)

	result, err := s.evaluationClient.GetAssessmentReport(ctx, assessmentID)
	if err != nil {
		log.Errorf("Failed to get assessment report via gRPC: %v", err)
		return nil, err
	}

	if result == nil {
		return nil, nil
	}

	dimensions := make([]DimensionInterpretResponse, len(result.Dimensions))
	for i, dim := range result.Dimensions {
		dimensions[i] = DimensionInterpretResponse{
			FactorCode:  dim.FactorCode,
			FactorName:  dim.FactorName,
			RawScore:    dim.RawScore,
			MaxScore:    dim.MaxScore,
			RiskLevel:   dim.RiskLevel,
			Description: dim.Description,
		}
	}

	return &AssessmentReportResponse{
		AssessmentID: strconv.FormatUint(result.AssessmentID, 10),
		ScaleCode:    result.ScaleCode,
		ScaleName:    result.ScaleName,
		TotalScore:   result.TotalScore,
		RiskLevel:    result.RiskLevel,
		Conclusion:   result.Conclusion,
		Dimensions:   dimensions,
		Suggestions:  result.Suggestions,
		CreatedAt:    result.CreatedAt,
	}, nil
}

// GetFactorTrend 获取因子得分趋势
func (s *QueryService) GetFactorTrend(ctx context.Context, testeeID uint64, req *GetFactorTrendRequest) ([]TrendPointResponse, error) {
	log.Infof("Getting factor trend: testeeID=%d, factorCode=%s", testeeID, req.FactorCode)

	if req.Limit <= 0 {
		req.Limit = 10
	}

	result, err := s.evaluationClient.GetFactorTrend(ctx, testeeID, req.FactorCode, req.Limit)
	if err != nil {
		log.Errorf("Failed to get factor trend via gRPC: %v", err)
		return nil, err
	}

	points := make([]TrendPointResponse, len(result))
	for i, point := range result {
		points[i] = TrendPointResponse{
			AssessmentID: strconv.FormatUint(point.AssessmentID, 10),
			Score:        point.Score,
			RiskLevel:    point.RiskLevel,
			CreatedAt:    point.CreatedAt,
		}
	}

	return points, nil
}

// GetHighRiskFactors 获取高风险因子
func (s *QueryService) GetHighRiskFactors(ctx context.Context, testeeID, assessmentID uint64) ([]FactorScoreResponse, error) {
	log.Infof("Getting high risk factors: testeeID=%d, assessmentID=%d", testeeID, assessmentID)

	result, err := s.evaluationClient.GetHighRiskFactors(ctx, testeeID, assessmentID)
	if err != nil {
		log.Errorf("Failed to get high risk factors via gRPC: %v", err)
		return nil, err
	}

	factors := make([]FactorScoreResponse, len(result))
	for i, f := range result {
		factors[i] = FactorScoreResponse{
			FactorCode:   f.FactorCode,
			FactorName:   f.FactorName,
			RawScore:     f.RawScore,
			RiskLevel:    f.RiskLevel,
			Conclusion:   f.Conclusion,
			Suggestion:   f.Suggestion,
			IsTotalScore: f.IsTotalScore,
		}
	}

	return factors, nil
}
