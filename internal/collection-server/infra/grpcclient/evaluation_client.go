package grpcclient

import (
	"context"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/evaluation"
)

// FactorScoreOutput 因子得分输出
type FactorScoreOutput struct {
	FactorCode   string
	FactorName   string
	RawScore     float64
	RiskLevel    string
	Conclusion   string
	Suggestion   string
	IsTotalScore bool
}

// SuggestionOutput 建议输出
type SuggestionOutput struct {
	Category   string
	Content    string
	FactorCode *string
}

// DimensionInterpretOutput 维度解读输出
type DimensionInterpretOutput struct {
	FactorCode  string
	FactorName  string
	RawScore    float64
	MaxScore    *float64
	RiskLevel   string
	Description string
	Suggestion  string
}

// TrendPointOutput 趋势数据点输出
type TrendPointOutput struct {
	AssessmentID uint64
	Score        float64
	RiskLevel    string
	CreatedAt    string
}

// EvaluationClient 测评服务 gRPC 客户端封装
type EvaluationClient struct {
	client     *Client
	grpcClient pb.EvaluationServiceClient
}

// NewEvaluationClient 创建测评服务客户端
func NewEvaluationClient(client *Client) *EvaluationClient {
	return &EvaluationClient{
		client:     client,
		grpcClient: pb.NewEvaluationServiceClient(client.Conn()),
	}
}

// GetAssessmentScores 获取测评得分详情
func (c *EvaluationClient) GetAssessmentScores(ctx context.Context, testeeID, assessmentID uint64) ([]FactorScoreOutput, error) {
	ctx, cancel := c.client.ContextWithTimeout(ctx)
	defer cancel()

	req := &pb.GetAssessmentScoresRequest{
		TesteeId:     testeeID,
		AssessmentId: assessmentID,
	}

	resp, err := c.grpcClient.GetAssessmentScores(ctx, req)
	if err != nil {
		return nil, err
	}

	scores := make([]FactorScoreOutput, len(resp.GetFactorScores()))
	for i, score := range resp.GetFactorScores() {
		scores[i] = FactorScoreOutput{
			FactorCode:   score.GetFactorCode(),
			FactorName:   score.GetFactorName(),
			RawScore:     score.GetRawScore(),
			RiskLevel:    score.GetRiskLevel(),
			Conclusion:   score.GetConclusion(),
			Suggestion:   score.GetSuggestion(),
			IsTotalScore: score.GetIsTotalScore(),
		}
	}

	return scores, nil
}

// GetFactorTrend 获取因子得分趋势
func (c *EvaluationClient) GetFactorTrend(ctx context.Context, testeeID uint64, factorCode string, limit int32) ([]TrendPointOutput, error) {
	ctx, cancel := c.client.ContextWithTimeout(ctx)
	defer cancel()

	req := &pb.GetFactorTrendRequest{
		TesteeId:   testeeID,
		FactorCode: factorCode,
		Limit:      limit,
	}

	resp, err := c.grpcClient.GetFactorTrend(ctx, req)
	if err != nil {
		return nil, err
	}

	points := make([]TrendPointOutput, len(resp.GetDataPoints()))
	for i, point := range resp.GetDataPoints() {
		points[i] = TrendPointOutput{
			AssessmentID: point.GetAssessmentId(),
			Score:        point.GetScore(),
			RiskLevel:    point.GetRiskLevel(),
			CreatedAt:    point.GetCreatedAt(),
		}
	}

	return points, nil
}

// GetHighRiskFactors 获取高风险因子
func (c *EvaluationClient) GetHighRiskFactors(ctx context.Context, testeeID, assessmentID uint64) ([]FactorScoreOutput, error) {
	ctx, cancel := c.client.ContextWithTimeout(ctx)
	defer cancel()

	req := &pb.GetHighRiskFactorsRequest{
		TesteeId:     testeeID,
		AssessmentId: assessmentID,
	}

	resp, err := c.grpcClient.GetHighRiskFactors(ctx, req)
	if err != nil {
		return nil, err
	}

	factors := make([]FactorScoreOutput, len(resp.GetHighRiskFactors()))
	for i, f := range resp.GetHighRiskFactors() {
		factors[i] = FactorScoreOutput{
			FactorCode:   f.GetFactorCode(),
			FactorName:   f.GetFactorName(),
			RawScore:     f.GetRawScore(),
			RiskLevel:    f.GetRiskLevel(),
			Conclusion:   f.GetConclusion(),
			Suggestion:   f.GetSuggestion(),
			IsTotalScore: f.GetIsTotalScore(),
		}
	}

	return factors, nil
}

// fromProtoSuggestions 从 proto 建议列表转换
func fromProtoSuggestions(protoSuggestions []*pb.Suggestion) []SuggestionOutput {
	if len(protoSuggestions) == 0 {
		return nil
	}
	result := make([]SuggestionOutput, len(protoSuggestions))
	for i, s := range protoSuggestions {
		suggestion := SuggestionOutput{
			Category: s.GetCategory(),
			Content:  s.GetContent(),
		}
		if s.GetFactorCode() != "" {
			fc := s.GetFactorCode()
			suggestion.FactorCode = &fc
		}
		result[i] = suggestion
	}
	return result
}
