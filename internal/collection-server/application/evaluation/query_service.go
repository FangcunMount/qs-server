package evaluation

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/log"
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
	log.Infof("Getting my assessment: testeeID=%d, assessmentID=%d", testeeID, assessmentID)

	result, err := s.evaluationClient.GetMyAssessment(ctx, testeeID, assessmentID)
	if err != nil {
		log.Errorf("Failed to get assessment via gRPC: %v", err)
		return nil, err
	}

	if result == nil {
		return nil, nil
	}

	return &AssessmentDetailResponse{
		ID:                   result.ID,
		OrgID:                result.OrgID,
		TesteeID:             result.TesteeID,
		QuestionnaireID:      result.QuestionnaireID,
		QuestionnaireCode:    result.QuestionnaireCode,
		QuestionnaireVersion: result.QuestionnaireVersion,
		AnswerSheetID:        result.AnswerSheetID,
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
	log.Infof("Listing my assessments: testeeID=%d, page=%d, pageSize=%d", testeeID, req.Page, req.PageSize)

	// 默认分页参数
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = 20
	}

	result, err := s.evaluationClient.ListMyAssessments(ctx, testeeID, req.Status, req.Page, req.PageSize)
	if err != nil {
		log.Errorf("Failed to list assessments via gRPC: %v", err)
		return nil, err
	}

	items := make([]AssessmentSummaryResponse, len(result.Items))
	for i, item := range result.Items {
		items[i] = AssessmentSummaryResponse{
			ID:                   item.ID,
			QuestionnaireID:      item.QuestionnaireID,
			QuestionnaireCode:    item.QuestionnaireCode,
			QuestionnaireVersion: item.QuestionnaireVersion,
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
	log.Infof("Getting assessment scores: testeeID=%d, assessmentID=%d", testeeID, assessmentID)

	result, err := s.evaluationClient.GetAssessmentScores(ctx, testeeID, assessmentID)
	if err != nil {
		log.Errorf("Failed to get assessment scores via gRPC: %v", err)
		return nil, err
	}

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
func (s *QueryService) GetAssessmentReport(ctx context.Context, testeeID, assessmentID uint64) (*AssessmentReportResponse, error) {
	log.Infof("Getting assessment report: testeeID=%d, assessmentID=%d", testeeID, assessmentID)

	result, err := s.evaluationClient.GetAssessmentReport(ctx, testeeID, assessmentID)
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
			RiskLevel:   dim.RiskLevel,
			Description: dim.Description,
		}
	}

	return &AssessmentReportResponse{
		AssessmentID: result.AssessmentID,
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
			AssessmentID: point.AssessmentID,
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
