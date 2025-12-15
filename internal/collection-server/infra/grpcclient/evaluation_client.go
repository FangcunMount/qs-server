package grpcclient

import (
	"context"

	pb "github.com/FangcunMount/qs-server/internal/apiserver/interface/grpc/proto/evaluation"
)

// ==================== Output Types ====================

// AssessmentSummaryOutput 测评摘要输出
type AssessmentSummaryOutput struct {
	ID                   uint64
	QuestionnaireCode    string
	QuestionnaireVersion string
	AnswerSheetID        uint64
	ScaleCode            string
	ScaleName            string
	OriginType           string
	Status               string
	TotalScore           float64
	RiskLevel            string
	CreatedAt            string
	SubmittedAt          string
	InterpretedAt        string
}

// AssessmentDetailOutput 测评详情输出
type AssessmentDetailOutput struct {
	ID                   uint64
	OrgID                uint64
	TesteeID             uint64
	QuestionnaireCode    string
	QuestionnaireVersion string
	AnswerSheetID        uint64
	ScaleCode            string
	ScaleName            string
	OriginType           string
	OriginID             string
	Status               string
	TotalScore           float64
	RiskLevel            string
	CreatedAt            string
	SubmittedAt          string
	InterpretedAt        string
	FailedAt             string
	FailureReason        string
}

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

// DimensionInterpretOutput 维度解读输出
type DimensionInterpretOutput struct {
	FactorCode  string
	FactorName  string
	RawScore    float64
	RiskLevel   string
	Description string
}

// AssessmentReportOutput 测评报告输出
type AssessmentReportOutput struct {
	AssessmentID uint64
	ScaleCode    string
	ScaleName    string
	TotalScore   float64
	RiskLevel    string
	Conclusion   string
	Dimensions   []DimensionInterpretOutput
	Suggestions  []string
	CreatedAt    string
}

// ListAssessmentsOutput 测评列表输出
type ListAssessmentsOutput struct {
	Items      []AssessmentSummaryOutput
	Total      int32
	Page       int32
	PageSize   int32
	TotalPages int32
}

// TrendPointOutput 趋势数据点输出
type TrendPointOutput struct {
	AssessmentID uint64
	Score        float64
	RiskLevel    string
	CreatedAt    string
}

// ==================== Client ====================

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

// GetMyAssessment 获取我的测评详情
func (c *EvaluationClient) GetMyAssessment(ctx context.Context, testeeID, assessmentID uint64) (*AssessmentDetailOutput, error) {
	ctx, cancel := c.client.ContextWithTimeout(ctx)
	defer cancel()

	req := &pb.GetMyAssessmentRequest{
		TesteeId:     testeeID,
		AssessmentId: assessmentID,
	}

	resp, err := c.grpcClient.GetMyAssessment(ctx, req)
	if err != nil {
		return nil, err
	}

	assessment := resp.GetAssessment()
	if assessment == nil {
		return nil, nil
	}

	return c.convertAssessmentDetail(assessment), nil
}

// GetMyAssessmentByAnswerSheetID 通过答卷ID获取测评详情
func (c *EvaluationClient) GetMyAssessmentByAnswerSheetID(ctx context.Context, answerSheetID uint64) (*AssessmentDetailOutput, error) {
	ctx, cancel := c.client.ContextWithTimeout(ctx)
	defer cancel()

	req := &pb.GetMyAssessmentByAnswerSheetIDRequest{
		AnswerSheetId: answerSheetID,
	}

	resp, err := c.grpcClient.GetMyAssessmentByAnswerSheetID(ctx, req)
	if err != nil {
		return nil, err
	}

	assessment := resp.GetAssessment()
	if assessment == nil {
		return nil, nil
	}

	return c.convertAssessmentDetail(assessment), nil
}

// ListMyAssessments 获取我的测评列表
func (c *EvaluationClient) ListMyAssessments(ctx context.Context, testeeID uint64, status string, page, pageSize int32) (*ListAssessmentsOutput, error) {
	ctx, cancel := c.client.ContextWithTimeout(ctx)
	defer cancel()

	req := &pb.ListMyAssessmentsRequest{
		TesteeId: testeeID,
		Status:   status,
		Page:     page,
		PageSize: pageSize,
	}

	resp, err := c.grpcClient.ListMyAssessments(ctx, req)
	if err != nil {
		return nil, err
	}

	items := make([]AssessmentSummaryOutput, len(resp.GetItems()))
	for i, item := range resp.GetItems() {
		items[i] = c.convertAssessmentSummary(item)
	}

	return &ListAssessmentsOutput{
		Items:      items,
		Total:      resp.GetTotal(),
		Page:       resp.GetPage(),
		PageSize:   resp.GetPageSize(),
		TotalPages: resp.GetTotalPages(),
	}, nil
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

// GetAssessmentReport 获取测评报告
func (c *EvaluationClient) GetAssessmentReport(ctx context.Context, assessmentID uint64) (*AssessmentReportOutput, error) {
	ctx, cancel := c.client.ContextWithTimeout(ctx)
	defer cancel()

	req := &pb.GetAssessmentReportRequest{
		AssessmentId: assessmentID,
	}

	resp, err := c.grpcClient.GetAssessmentReport(ctx, req)
	if err != nil {
		return nil, err
	}

	report := resp.GetReport()
	if report == nil {
		return nil, nil
	}

	dimensions := make([]DimensionInterpretOutput, len(report.GetDimensions()))
	for i, dim := range report.GetDimensions() {
		dimensions[i] = DimensionInterpretOutput{
			FactorCode:  dim.GetFactorCode(),
			FactorName:  dim.GetFactorName(),
			RawScore:    dim.GetRawScore(),
			RiskLevel:   dim.GetRiskLevel(),
			Description: dim.GetDescription(),
		}
	}

	return &AssessmentReportOutput{
		AssessmentID: report.GetAssessmentId(),
		ScaleCode:    report.GetScaleCode(),
		ScaleName:    report.GetScaleName(),
		TotalScore:   report.GetTotalScore(),
		RiskLevel:    report.GetRiskLevel(),
		Conclusion:   report.GetConclusion(),
		Dimensions:   dimensions,
		Suggestions:  report.GetSuggestions(),
		CreatedAt:    report.GetCreatedAt(),
	}, nil
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

// ==================== Helpers ====================

func (c *EvaluationClient) convertAssessmentDetail(a *pb.AssessmentDetail) *AssessmentDetailOutput {
	return &AssessmentDetailOutput{
		ID:                   a.GetId(),
		OrgID:                a.GetOrgId(),
		TesteeID:             a.GetTesteeId(),
		QuestionnaireCode:    a.GetQuestionnaireCode(),
		QuestionnaireVersion: a.GetQuestionnaireVersion(),
		AnswerSheetID:        a.GetAnswerSheetId(),
		ScaleCode:            a.GetScaleCode(),
		ScaleName:            a.GetScaleName(),
		OriginType:           a.GetOriginType(),
		OriginID:             a.GetOriginId(),
		Status:               a.GetStatus(),
		TotalScore:           a.GetTotalScore(),
		RiskLevel:            a.GetRiskLevel(),
		CreatedAt:            a.GetCreatedAt(),
		SubmittedAt:          a.GetSubmittedAt(),
		InterpretedAt:        a.GetInterpretedAt(),
		FailedAt:             a.GetFailedAt(),
		FailureReason:        a.GetFailureReason(),
	}
}

func (c *EvaluationClient) convertAssessmentSummary(a *pb.AssessmentSummary) AssessmentSummaryOutput {
	return AssessmentSummaryOutput{
		ID:                   a.GetId(),
		QuestionnaireCode:    a.GetQuestionnaireCode(),
		QuestionnaireVersion: a.GetQuestionnaireVersion(),
		AnswerSheetID:        a.GetAnswerSheetId(),
		ScaleCode:            a.GetScaleCode(),
		ScaleName:            a.GetScaleName(),
		OriginType:           a.GetOriginType(),
		Status:               a.GetStatus(),
		TotalScore:           a.GetTotalScore(),
		RiskLevel:            a.GetRiskLevel(),
		CreatedAt:            a.GetCreatedAt(),
		SubmittedAt:          a.GetSubmittedAt(),
		InterpretedAt:        a.GetInterpretedAt(),
	}
}
