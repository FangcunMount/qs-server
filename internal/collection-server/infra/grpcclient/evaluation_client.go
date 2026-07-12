package grpcclient

import (
	"context"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/evaluation"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/answersheet"
)

// FactorScoreOutput 因子得分输出
type FactorScoreOutput struct {
	FactorCode   string
	FactorName   string
	RawScore     float64
	RiskLevel    string
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
	client       *Client
	grpcClient   pb.TesteeEvaluationServiceClient
	reportClient pb.ParticipantReportServiceClient
	intakeClient pb.AssessmentIntakeServiceClient
}

// NewEvaluationClient 创建测评服务客户端
func NewEvaluationClient(client *Client) *EvaluationClient {
	return &EvaluationClient{
		client:       client,
		grpcClient:   pb.NewTesteeEvaluationServiceClient(client.Conn()),
		reportClient: pb.NewParticipantReportServiceClient(client.Conn()),
		intakeClient: pb.NewAssessmentIntakeServiceClient(client.Conn()),
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

type ModelIdentityOutput struct {
	Kind            string
	SubKind         string
	Algorithm       string
	Code            string
	Version         string
	Title           string
	ProductChannel  string
	AlgorithmFamily string
}

type ScoreValueOutput struct {
	Kind  string
	Value float64
	Label string
	Max   *float64
}

type ResultLevelOutput struct {
	Code     string
	Label    string
	Severity string
}

type ModelExtraOutput struct {
	Kind           string
	TypeCode       string
	TypeName       string
	OneLiner       string
	ImageURL       string
	MatchPercent   float64
	IsSpecial      bool
	SpecialTrigger string
	Commentary     string
	Rarity         *ModelRarityOutput
}

type ModelRarityOutput struct {
	Percent float64
	Label   string
	OneInX  int32
}

type AssessmentDetailOutput struct {
	ID                   uint64
	OrgID                uint64
	TesteeID             uint64
	QuestionnaireCode    string
	QuestionnaireVersion string
	AnswerSheetID        uint64
	Model                ModelIdentityOutput
	PrimaryScore         *ScoreValueOutput
	Level                *ResultLevelOutput
	OriginType           string
	OriginID             string
	Status               string
	SubmittedAt          string
	InterpretedAt        string
	FailedAt             string
	FailureReason        string
}

type AssessmentSummaryOutput struct {
	ID                   uint64
	QuestionnaireCode    string
	QuestionnaireVersion string
	AnswerSheetID        uint64
	Model                ModelIdentityOutput
	PrimaryScore         *ScoreValueOutput
	Level                *ResultLevelOutput
	OriginType           string
	Status               string
	SubmittedAt          string
	InterpretedAt        string
}

type AssessmentReportOutput struct {
	AssessmentID uint64
	Model        ModelIdentityOutput
	PrimaryScore *ScoreValueOutput
	Level        *ResultLevelOutput
	Conclusion   string
	Dimensions   []DimensionInterpretOutput
	Suggestions  []SuggestionOutput
	ModelExtra   *ModelExtraOutput
	CreatedAt    string
}

type ListAssessmentsOutput struct {
	Items      []AssessmentSummaryOutput
	Total      int32
	Page       int32
	PageSize   int32
	TotalPages int32
}

func (c *EvaluationClient) GetMyAssessment(ctx context.Context, testeeID, assessmentID uint64) (*AssessmentDetailOutput, error) {
	ctx, cancel := c.client.ContextWithTimeout(ctx)
	defer cancel()

	resp, err := c.grpcClient.GetMyAssessment(ctx, &pb.GetMyAssessmentRequest{
		TesteeId:     testeeID,
		AssessmentId: assessmentID,
	})
	if err != nil {
		return nil, err
	}
	return convertAssessmentDetail(resp.GetAssessment()), nil
}

func (c *EvaluationClient) ListMyAssessments(
	ctx context.Context,
	testeeID uint64,
	status, scaleCode, riskLevel, dateFrom, dateTo, modelKind string,
	page, pageSize int32,
) (*ListAssessmentsOutput, error) {
	ctx, cancel := c.client.ContextWithTimeout(ctx)
	defer cancel()

	resp, err := c.grpcClient.ListMyAssessments(ctx, &pb.ListMyAssessmentsRequest{
		TesteeId:  testeeID,
		Status:    status,
		Page:      page,
		PageSize:  pageSize,
		ScaleCode: scaleCode,
		RiskLevel: riskLevel,
		DateFrom:  dateFrom,
		DateTo:    dateTo,
		ModelKind: modelKind,
	})
	if err != nil {
		return nil, err
	}
	items := make([]AssessmentSummaryOutput, 0, len(resp.GetItems()))
	for _, item := range resp.GetItems() {
		items = append(items, convertAssessmentSummary(item))
	}
	return &ListAssessmentsOutput{
		Items:      items,
		Total:      resp.GetTotal(),
		Page:       resp.GetPage(),
		PageSize:   resp.GetPageSize(),
		TotalPages: resp.GetTotalPages(),
	}, nil
}

func (c *EvaluationClient) GetAssessmentReport(ctx context.Context, testeeID, assessmentID uint64) (*AssessmentReportOutput, error) {
	ctx, cancel := c.client.ContextWithTimeout(ctx)
	defer cancel()

	resp, err := c.reportClient.GetAssessmentReport(ctx, &pb.GetAssessmentReportRequest{
		AssessmentId: assessmentID,
		TesteeId:     testeeID,
	})
	if err != nil {
		return nil, err
	}
	return convertAssessmentReport(resp.GetReport()), nil
}

// ResolveAssessmentByAnswerSheetID resolves ownership keys for a legacy answer-sheet lookup RPC.
func (c *EvaluationClient) ResolveAssessmentByAnswerSheetID(ctx context.Context, answerSheetID uint64) (testeeID, assessmentID uint64, err error) {
	ctx, cancel := c.client.ContextWithTimeout(ctx)
	defer cancel()

	resp, err := c.intakeClient.ResolveAssessmentByAnswerSheetID(ctx, &pb.ResolveAssessmentByAnswerSheetIDRequest{
		AnswerSheetId: answerSheetID,
	})
	if err != nil {
		return 0, 0, err
	}
	return resp.GetTesteeId(), resp.GetAssessmentId(), nil
}

func (c *EvaluationClient) EnsureAssessment(ctx context.Context, input answersheet.EnsureAssessmentInput) (uint64, error) {
	ctx, cancel := c.client.ContextWithTimeout(ctx)
	defer cancel()
	resp, err := c.intakeClient.EnsureAssessment(ctx, &pb.EnsureAssessmentRequest{OrgId: input.OrgID, AnswerSheetId: input.AnswerSheetID, QuestionnaireCode: input.QuestionnaireCode, QuestionnaireVersion: input.QuestionnaireVersion, TesteeId: input.TesteeID, FillerId: input.FillerID, TaskId: input.TaskID})
	if err != nil {
		return 0, err
	}
	return resp.GetAssessmentId(), nil
}

func convertAssessmentDetail(assessment *pb.AssessmentDetail) *AssessmentDetailOutput {
	if assessment == nil {
		return nil
	}
	return &AssessmentDetailOutput{
		ID:                   assessment.GetId(),
		OrgID:                assessment.GetOrgId(),
		TesteeID:             assessment.GetTesteeId(),
		QuestionnaireCode:    assessment.GetQuestionnaireCode(),
		QuestionnaireVersion: assessment.GetQuestionnaireVersion(),
		AnswerSheetID:        assessment.GetAnswerSheetId(),
		Model:                convertModelIdentity(assessment.GetModel()),
		PrimaryScore:         convertScoreValue(assessment.GetPrimaryScore()),
		Level:                convertResultLevel(assessment.GetLevel()),
		OriginType:           assessment.GetOriginType(),
		OriginID:             assessment.GetOriginId(),
		Status:               assessment.GetStatus(),
		SubmittedAt:          assessment.GetSubmittedAt(),
		FailedAt:             assessment.GetFailedAt(),
		FailureReason:        assessment.GetFailureReason(),
	}
}

func convertAssessmentSummary(summary *pb.AssessmentSummary) AssessmentSummaryOutput {
	if summary == nil {
		return AssessmentSummaryOutput{}
	}
	return AssessmentSummaryOutput{
		ID:                   summary.GetId(),
		QuestionnaireCode:    summary.GetQuestionnaireCode(),
		QuestionnaireVersion: summary.GetQuestionnaireVersion(),
		AnswerSheetID:        summary.GetAnswerSheetId(),
		Model:                convertModelIdentity(summary.GetModel()),
		PrimaryScore:         convertScoreValue(summary.GetPrimaryScore()),
		Level:                convertResultLevel(summary.GetLevel()),
		OriginType:           summary.GetOriginType(),
		Status:               summary.GetStatus(),
		SubmittedAt:          summary.GetSubmittedAt(),
	}
}

func convertAssessmentReport(report *pb.AssessmentReport) *AssessmentReportOutput {
	if report == nil {
		return nil
	}
	dimensions := make([]DimensionInterpretOutput, 0, len(report.GetDimensions()))
	for _, dim := range report.GetDimensions() {
		var maxScore *float64
		if dim.GetMaxScore() != 0 {
			score := dim.GetMaxScore()
			maxScore = &score
		}
		dimensions = append(dimensions, DimensionInterpretOutput{
			FactorCode:  dim.GetFactorCode(),
			FactorName:  dim.GetFactorName(),
			RawScore:    dim.GetRawScore(),
			MaxScore:    maxScore,
			RiskLevel:   dim.GetRiskLevel(),
			Description: dim.GetDescription(),
			Suggestion:  dim.GetSuggestion(),
		})
	}
	return &AssessmentReportOutput{
		AssessmentID: report.GetAssessmentId(),
		Model:        convertModelIdentity(report.GetModel()),
		PrimaryScore: convertScoreValue(report.GetPrimaryScore()),
		Level:        convertResultLevel(report.GetLevel()),
		Conclusion:   report.GetConclusion(),
		Dimensions:   dimensions,
		Suggestions:  fromProtoSuggestions(report.GetSuggestions()),
		ModelExtra:   convertModelExtra(report.GetModelExtra()),
		CreatedAt:    report.GetCreatedAt(),
	}
}

func convertModelIdentity(model *pb.ModelIdentity) ModelIdentityOutput {
	if model == nil {
		return ModelIdentityOutput{}
	}
	return ModelIdentityOutput{
		Kind:            model.GetKind(),
		SubKind:         model.GetSubKind(),
		Algorithm:       model.GetAlgorithm(),
		Code:            model.GetCode(),
		Version:         model.GetVersion(),
		Title:           model.GetTitle(),
		ProductChannel:  model.GetProductChannel(),
		AlgorithmFamily: model.GetAlgorithmFamily(),
	}
}

func convertScoreValue(score *pb.ScoreValue) *ScoreValueOutput {
	if score == nil {
		return nil
	}
	var max *float64
	if score.GetMax() != 0 {
		value := score.GetMax()
		max = &value
	}
	return &ScoreValueOutput{
		Kind:  score.GetKind(),
		Value: score.GetValue(),
		Label: score.GetLabel(),
		Max:   max,
	}
}

func convertResultLevel(level *pb.ResultLevel) *ResultLevelOutput {
	if level == nil {
		return nil
	}
	return &ResultLevelOutput{
		Code:     level.GetCode(),
		Label:    level.GetLabel(),
		Severity: level.GetSeverity(),
	}
}

func convertModelExtra(extra *pb.ModelExtra) *ModelExtraOutput {
	if extra == nil {
		return nil
	}
	out := &ModelExtraOutput{
		Kind:           extra.GetKind(),
		TypeCode:       extra.GetTypeCode(),
		TypeName:       extra.GetTypeName(),
		OneLiner:       extra.GetOneLiner(),
		ImageURL:       extra.GetImageUrl(),
		MatchPercent:   extra.GetMatchPercent(),
		IsSpecial:      extra.GetIsSpecial(),
		SpecialTrigger: extra.GetSpecialTrigger(),
		Commentary:     extra.GetCommentary(),
	}
	if rarity := extra.GetRarity(); rarity != nil {
		out.Rarity = &ModelRarityOutput{
			Percent: rarity.GetPercent(),
			Label:   rarity.GetLabel(),
			OneInX:  rarity.GetOneInX(),
		}
	}
	return out
}
