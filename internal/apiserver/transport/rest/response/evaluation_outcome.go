package response

import (
	"fmt"

	assessment "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/assessment"
)

// ModelIdentityResponse is the outcome published-model reference on REST responses.
type ModelIdentityResponse struct {
	// 测评层 kind；人格线当前输出 personality，读兼容 typology。
	Kind            string `json:"kind" example:"personality" enums:"personality,typology"`
	SubKind         string `json:"sub_kind,omitempty" example:"typology"`
	Algorithm       string `json:"algorithm,omitempty"`
	Code            string `json:"code"`
	Version         string `json:"version,omitempty"`
	Title           string `json:"title,omitempty"`
	ProductChannel  string `json:"product_channel,omitempty"`
	AlgorithmFamily string `json:"algorithm_family,omitempty"`
}

// ScoreValueResponse is the outcome primary score projection.
type ScoreValueResponse struct {
	Kind  string   `json:"kind"`
	Value float64  `json:"value"`
	Label string   `json:"label,omitempty"`
	Max   *float64 `json:"max,omitempty"`
}

// ResultLevelResponse is the outcome level projection.
type ResultLevelResponse struct {
	Code     string `json:"code"`
	Label    string `json:"label"`
	Severity string `json:"severity,omitempty"`
}

// AssessmentOutcomeResponse exposes assessment facts with outcome summary.
type AssessmentOutcomeResponse struct {
	ID                   string                `json:"id"`
	OrgID                string                `json:"org_id"`
	TesteeID             string                `json:"testee_id"`
	QuestionnaireCode    string                `json:"questionnaire_code"`
	QuestionnaireVersion string                `json:"questionnaire_version"`
	AnswerSheetID        string                `json:"answer_sheet_id"`
	Model                ModelIdentityResponse `json:"model"`
	PrimaryScore         *ScoreValueResponse   `json:"primary_score,omitempty"`
	Level                *ResultLevelResponse  `json:"level,omitempty"`
	OriginType           string                `json:"origin_type"`
	OriginTypeLabel      string                `json:"origin_type_label,omitempty"`
	OriginID             *string               `json:"origin_id,omitempty"`
	Status               string                `json:"status"`
	StatusLabel          string                `json:"status_label,omitempty"`
	SubmittedAt          *string               `json:"submitted_at,omitempty"`
	InterpretedAt        *string               `json:"interpreted_at,omitempty"`
	FailedAt             *string               `json:"failed_at,omitempty"`
	FailureReason        *string               `json:"failure_reason,omitempty"`
}

// AssessmentOutcomeListResponse is a paginated outcome assessment list.
type AssessmentOutcomeListResponse struct {
	Items      []*AssessmentOutcomeResponse `json:"items"`
	Total      int                          `json:"total"`
	Page       int                          `json:"page"`
	PageSize   int                          `json:"page_size"`
	TotalPages int                          `json:"total_pages"`
}

// ReportOutcomeResponse exposes report facts with outcome summary.
type ReportOutcomeResponse struct {
	AssessmentID string                `json:"assessment_id"`
	Model        ModelIdentityResponse `json:"model"`
	PrimaryScore *ScoreValueResponse   `json:"primary_score,omitempty"`
	Level        *ResultLevelResponse  `json:"level,omitempty"`
	Conclusion   string                `json:"conclusion"`
	Dimensions   []*DimensionItem      `json:"dimensions"`
	Suggestions  []SuggestionItem      `json:"suggestions"`
	ModelExtra   *ModelExtraResponse   `json:"model_extra,omitempty"`
	CreatedAt    string                `json:"created_at"`
}

// ModelExtraResponse carries typology-specific report extensions.
type ModelExtraResponse struct {
	Kind           string               `json:"kind,omitempty"`
	TypeCode       string               `json:"type_code,omitempty"`
	TypeName       string               `json:"type_name,omitempty"`
	OneLiner       string               `json:"one_liner,omitempty"`
	ImageURL       string               `json:"image_url,omitempty"`
	MatchPercent   float64              `json:"match_percent,omitempty"`
	IsSpecial      bool                 `json:"is_special,omitempty"`
	SpecialTrigger string               `json:"special_trigger,omitempty"`
	Commentary     string               `json:"commentary,omitempty"`
	Rarity         *ModelRarityResponse `json:"rarity,omitempty"`
}

// ModelRarityResponse is the theoretical rarity projection.
type ModelRarityResponse struct {
	Percent float64 `json:"percent,omitempty"`
	Label   string  `json:"label,omitempty"`
	OneInX  int     `json:"one_in_x,omitempty"`
}

// ReportOutcomeListResponse is a paginated outcome report list.
type ReportOutcomeListResponse struct {
	Items      []*ReportOutcomeResponse `json:"items"`
	Total      int                      `json:"total"`
	Page       int                      `json:"page"`
	PageSize   int                      `json:"page_size"`
	TotalPages int                      `json:"total_pages"`
}

// NewAssessmentOutcomeResponse maps application outcome result to REST response.
func NewAssessmentOutcomeResponse(result *assessment.AssessmentOutcomeResult) *AssessmentOutcomeResponse {
	if result == nil {
		return nil
	}
	resp := &AssessmentOutcomeResponse{
		ID:                   fmt.Sprintf("%d", result.ID),
		OrgID:                fmt.Sprintf("%d", result.OrgID),
		TesteeID:             fmt.Sprintf("%d", result.TesteeID),
		QuestionnaireCode:    result.QuestionnaireCode,
		QuestionnaireVersion: result.QuestionnaireVersion,
		AnswerSheetID:        fmt.Sprintf("%d", result.AnswerSheetID),
		Model:                newModelIdentityResponse(result.Model),
		PrimaryScore:         newScoreValueResponse(result.PrimaryScore),
		Level:                newResultLevelResponse(result.Level),
		OriginType:           result.OriginType,
		OriginTypeLabel:      LabelForAssessmentOriginType(result.OriginType),
		OriginID:             result.OriginID,
		Status:               result.Status,
		StatusLabel:          LabelForAssessmentStatus(result.Status),
		FailureReason:        result.FailureReason,
	}
	if result.SubmittedAt != nil {
		resp.SubmittedAt = FormatDateTimePtr(result.SubmittedAt)
	}
	if result.InterpretedAt != nil {
		resp.InterpretedAt = FormatDateTimePtr(result.InterpretedAt)
	}
	if result.FailedAt != nil {
		resp.FailedAt = FormatDateTimePtr(result.FailedAt)
	}
	return resp
}

// NewAssessmentOutcomeListResponse maps application outcome list result to REST response.
func NewAssessmentOutcomeListResponse(result *assessment.AssessmentOutcomeListResult) *AssessmentOutcomeListResponse {
	if result == nil {
		return nil
	}
	items := make([]*AssessmentOutcomeResponse, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, NewAssessmentOutcomeResponse(item))
	}
	return &AssessmentOutcomeListResponse{
		Items:      items,
		Total:      result.Total,
		Page:       result.Page,
		PageSize:   result.PageSize,
		TotalPages: result.TotalPages,
	}
}

// NewReportOutcomeResponse maps application outcome report result to REST response.
func NewReportOutcomeResponse(result *assessment.ReportOutcomeResult) *ReportOutcomeResponse {
	if result == nil {
		return nil
	}
	dimensions := make([]*DimensionItem, 0, len(result.Dimensions))
	for _, d := range result.Dimensions {
		dimensions = append(dimensions, newDimensionItem(d))
	}
	var modelExtra *ModelExtraResponse
	if result.ModelExtra != nil {
		modelExtra = newModelExtraResponse(result.ModelExtra)
	}
	return &ReportOutcomeResponse{
		AssessmentID: fmt.Sprintf("%d", result.AssessmentID),
		Model:        newModelIdentityResponse(result.Model),
		PrimaryScore: newScoreValueResponse(result.PrimaryScore),
		Level:        newResultLevelResponse(result.Level),
		Conclusion:   result.Conclusion,
		Dimensions:   dimensions,
		Suggestions:  toSuggestionItems(result.Suggestions),
		ModelExtra:   modelExtra,
		CreatedAt:    FormatDateTimeValue(result.CreatedAt),
	}
}

// NewReportOutcomeListResponse maps application outcome report list to REST response.
func NewReportOutcomeListResponse(result *assessment.ReportOutcomeListResult) *ReportOutcomeListResponse {
	if result == nil {
		return nil
	}
	items := make([]*ReportOutcomeResponse, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, NewReportOutcomeResponse(item))
	}
	return &ReportOutcomeListResponse{
		Items:      items,
		Total:      result.Total,
		Page:       result.Page,
		PageSize:   result.PageSize,
		TotalPages: result.TotalPages,
	}
}

func newModelIdentityResponse(model assessment.ModelIdentityResult) ModelIdentityResponse {
	return ModelIdentityResponse{
		Kind:            model.Kind,
		SubKind:         model.SubKind,
		Algorithm:       model.Algorithm,
		Code:            model.Code,
		Version:         model.Version,
		Title:           model.Title,
		ProductChannel:  model.ProductChannel,
		AlgorithmFamily: model.AlgorithmFamily,
	}
}

func newScoreValueResponse(score *assessment.ScoreValueResult) *ScoreValueResponse {
	if score == nil {
		return nil
	}
	return &ScoreValueResponse{
		Kind:  score.Kind,
		Value: score.Value,
		Label: score.Label,
		Max:   score.Max,
	}
}

func newResultLevelResponse(level *assessment.ResultLevelResult) *ResultLevelResponse {
	if level == nil {
		return nil
	}
	return &ResultLevelResponse{
		Code:     level.Code,
		Label:    level.Label,
		Severity: level.Severity,
	}
}

func newModelExtraResponse(extra *assessment.ModelExtraResult) *ModelExtraResponse {
	if extra == nil {
		return nil
	}
	resp := &ModelExtraResponse{
		Kind:           extra.Kind,
		TypeCode:       extra.TypeCode,
		TypeName:       extra.TypeName,
		OneLiner:       extra.OneLiner,
		ImageURL:       extra.ImageURL,
		MatchPercent:   extra.MatchPercent,
		IsSpecial:      extra.IsSpecial,
		SpecialTrigger: extra.SpecialTrigger,
		Commentary:     extra.Commentary,
	}
	if extra.Rarity != nil {
		resp.Rarity = &ModelRarityResponse{
			Percent: extra.Rarity.Percent,
			Label:   extra.Rarity.Label,
			OneInX:  extra.Rarity.OneInX,
		}
	}
	return resp
}
