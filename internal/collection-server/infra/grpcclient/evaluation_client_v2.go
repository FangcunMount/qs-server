package grpcclient

import (
	"context"

	pb "github.com/FangcunMount/qs-server/internal/apiserver/interface/grpc/proto/evaluation"
)

type ModelIdentityOutput struct {
	Kind      string
	SubKind   string
	Algorithm string
	Code      string
	Version   string
	Title     string
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

type AssessmentDetailV2Output struct {
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

type AssessmentSummaryV2Output struct {
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

type AssessmentReportV2Output struct {
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

type ListAssessmentsV2Output struct {
	Items      []AssessmentSummaryV2Output
	Total      int32
	Page       int32
	PageSize   int32
	TotalPages int32
}

func (c *EvaluationClient) GetMyAssessmentV2(ctx context.Context, testeeID, assessmentID uint64) (*AssessmentDetailV2Output, error) {
	ctx, cancel := c.client.ContextWithTimeout(ctx)
	defer cancel()

	resp, err := c.grpcClient.GetMyAssessmentV2(ctx, &pb.GetMyAssessmentV2Request{
		TesteeId:     testeeID,
		AssessmentId: assessmentID,
	})
	if err != nil {
		return nil, err
	}
	return convertAssessmentDetailV2(resp.GetAssessment()), nil
}

func (c *EvaluationClient) ListMyAssessmentsV2(
	ctx context.Context,
	testeeID uint64,
	status, scaleCode, riskLevel, modelKind, modelAlgorithm string,
	page, pageSize int32,
) (*ListAssessmentsV2Output, error) {
	ctx, cancel := c.client.ContextWithTimeout(ctx)
	defer cancel()

	resp, err := c.grpcClient.ListMyAssessmentsV2(ctx, &pb.ListMyAssessmentsV2Request{
		TesteeId:       testeeID,
		Status:         status,
		Page:           page,
		PageSize:       pageSize,
		ScaleCode:      scaleCode,
		RiskLevel:      riskLevel,
		ModelKind:      modelKind,
		ModelAlgorithm: modelAlgorithm,
	})
	if err != nil {
		return nil, err
	}
	items := make([]AssessmentSummaryV2Output, 0, len(resp.GetItems()))
	for _, item := range resp.GetItems() {
		items = append(items, convertAssessmentSummaryV2(item))
	}
	return &ListAssessmentsV2Output{
		Items:      items,
		Total:      resp.GetTotal(),
		Page:       resp.GetPage(),
		PageSize:   resp.GetPageSize(),
		TotalPages: resp.GetTotalPages(),
	}, nil
}

func (c *EvaluationClient) GetAssessmentReportV2(ctx context.Context, assessmentID uint64) (*AssessmentReportV2Output, error) {
	ctx, cancel := c.client.ContextWithTimeout(ctx)
	defer cancel()

	resp, err := c.grpcClient.GetAssessmentReportV2(ctx, &pb.GetAssessmentReportV2Request{
		AssessmentId: assessmentID,
	})
	if err != nil {
		return nil, err
	}
	return convertAssessmentReportV2(resp.GetReport()), nil
}

func convertAssessmentDetailV2(assessment *pb.AssessmentDetailV2) *AssessmentDetailV2Output {
	if assessment == nil {
		return nil
	}
	return &AssessmentDetailV2Output{
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
		InterpretedAt:        assessment.GetInterpretedAt(),
		FailedAt:             assessment.GetFailedAt(),
		FailureReason:        assessment.GetFailureReason(),
	}
}

func convertAssessmentSummaryV2(summary *pb.AssessmentSummaryV2) AssessmentSummaryV2Output {
	if summary == nil {
		return AssessmentSummaryV2Output{}
	}
	return AssessmentSummaryV2Output{
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
		InterpretedAt:        summary.GetInterpretedAt(),
	}
}

func convertAssessmentReportV2(report *pb.AssessmentReportV2) *AssessmentReportV2Output {
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
	return &AssessmentReportV2Output{
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
		Kind:      model.GetKind(),
		SubKind:   model.GetSubKind(),
		Algorithm: model.GetAlgorithm(),
		Code:      model.GetCode(),
		Version:   model.GetVersion(),
		Title:     model.GetTitle(),
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
