package response

import (
	"fmt"

	assessment "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/assessment"
)

// ModelIdentityResponse is the v2 published-model reference on REST responses.
type ModelIdentityResponse struct {
	Kind      string `json:"kind"`
	SubKind   string `json:"sub_kind,omitempty"`
	Algorithm string `json:"algorithm,omitempty"`
	Code      string `json:"code"`
	Version   string `json:"version,omitempty"`
	Title     string `json:"title,omitempty"`
}

// ScoreValueResponse is the v2 primary score projection.
type ScoreValueResponse struct {
	Kind  string   `json:"kind"`
	Value float64  `json:"value"`
	Label string   `json:"label,omitempty"`
	Max   *float64 `json:"max,omitempty"`
}

// ResultLevelResponse is the v2 outcome level projection.
type ResultLevelResponse struct {
	Code     string `json:"code"`
	Label    string `json:"label"`
	Severity string `json:"severity,omitempty"`
}

// AssessmentV2Response exposes assessment facts with v2 outcome summary.
type AssessmentV2Response struct {
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

// AssessmentV2ListResponse is a paginated v2 assessment list.
type AssessmentV2ListResponse struct {
	Items      []*AssessmentV2Response `json:"items"`
	Total      int                     `json:"total"`
	Page       int                     `json:"page"`
	PageSize   int                     `json:"page_size"`
	TotalPages int                     `json:"total_pages"`
}

// ReportV2Response exposes report facts with v2 outcome summary.
type ReportV2Response struct {
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
	TypeCode string `json:"type_code,omitempty"`
}

// ReportV2ListResponse is a paginated v2 report list.
type ReportV2ListResponse struct {
	Items      []*ReportV2Response `json:"items"`
	Total      int                 `json:"total"`
	Page       int                 `json:"page"`
	PageSize   int                 `json:"page_size"`
	TotalPages int                 `json:"total_pages"`
}

// NewAssessmentV2Response maps application v2 result to REST response.
func NewAssessmentV2Response(result *assessment.AssessmentV2Result) *AssessmentV2Response {
	if result == nil {
		return nil
	}
	resp := &AssessmentV2Response{
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

// NewAssessmentV2ListResponse maps application v2 list result to REST response.
func NewAssessmentV2ListResponse(result *assessment.AssessmentV2ListResult) *AssessmentV2ListResponse {
	if result == nil {
		return nil
	}
	items := make([]*AssessmentV2Response, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, NewAssessmentV2Response(item))
	}
	return &AssessmentV2ListResponse{
		Items:      items,
		Total:      result.Total,
		Page:       result.Page,
		PageSize:   result.PageSize,
		TotalPages: result.TotalPages,
	}
}

// NewReportV2Response maps application v2 report result to REST response.
func NewReportV2Response(result *assessment.ReportV2Result) *ReportV2Response {
	if result == nil {
		return nil
	}
	dimensions := make([]*DimensionItem, 0, len(result.Dimensions))
	for _, d := range result.Dimensions {
		dimensions = append(dimensions, &DimensionItem{
			FactorCode:     d.FactorCode,
			FactorName:     d.FactorName,
			RawScore:       d.RawScore,
			MaxScore:       d.MaxScore,
			RiskLevel:      d.RiskLevel,
			RiskLevelLabel: LabelForRiskLevel(d.RiskLevel),
			Description:    d.Description,
			Suggestion:     d.Suggestion,
		})
	}
	var modelExtra *ModelExtraResponse
	if result.ModelExtra != nil {
		modelExtra = &ModelExtraResponse{TypeCode: result.ModelExtra.TypeCode}
	}
	return &ReportV2Response{
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

// NewReportV2ListResponse maps application v2 report list to REST response.
func NewReportV2ListResponse(result *assessment.ReportV2ListResult) *ReportV2ListResponse {
	if result == nil {
		return nil
	}
	items := make([]*ReportV2Response, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, NewReportV2Response(item))
	}
	return &ReportV2ListResponse{
		Items:      items,
		Total:      result.Total,
		Page:       result.Page,
		PageSize:   result.PageSize,
		TotalPages: result.TotalPages,
	}
}

func newModelIdentityResponse(model assessment.ModelIdentityResult) ModelIdentityResponse {
	return ModelIdentityResponse{
		Kind:      model.Kind,
		SubKind:   model.SubKind,
		Algorithm: model.Algorithm,
		Code:      model.Code,
		Version:   model.Version,
		Title:     model.Title,
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
