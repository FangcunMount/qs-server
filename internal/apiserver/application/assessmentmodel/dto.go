package assessmentmodel

import "encoding/json"

const (
	KindPersonality      = "personality"
	KindBehaviorAbility  = "behavior_ability"
	StatusDraft          = "draft"
	StatusPublished      = "published"
	StatusArchived       = "archived"
	PayloadFormatScaleV1 = "assessmentmodel.behavior_ability.scale.v1"
)

type Option struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

type ListModelsDTO struct {
	Kind     string
	Status   string
	Keyword  string
	Category string
	Page     int
	PageSize int
}

type CreateModelDTO struct {
	Code                 string
	Kind                 string
	Title                string
	Description          string
	Category             string
	Tags                 []string
	QuestionnaireCode    string
	QuestionnaireVersion string
}

type UpdateBasicInfoDTO struct {
	Code        string
	Title       string
	Description string
	Category    string
	Tags        []string
}

type BindQuestionnaireDTO struct {
	Code                 string
	QuestionnaireCode    string
	QuestionnaireVersion string
}

type DefinitionDTO struct {
	Kind          string          `json:"kind"`
	SubKind       string          `json:"sub_kind,omitempty"`
	Algorithm     string          `json:"algorithm,omitempty"`
	PayloadFormat string          `json:"payload_format"`
	Payload       json.RawMessage `json:"payload"`
}

type ApplyCodesDTO struct {
	Code   string
	Target string
	Count  int
}

type ModelSummary struct {
	Code                 string   `json:"code"`
	Kind                 string   `json:"kind"`
	Title                string   `json:"title"`
	Description          string   `json:"description,omitempty"`
	Status               string   `json:"status"`
	Category             string   `json:"category,omitempty"`
	Tags                 []string `json:"tags,omitempty"`
	QuestionnaireCode    string   `json:"questionnaire_code,omitempty"`
	QuestionnaireVersion string   `json:"questionnaire_version,omitempty"`
	CreatedAt            string   `json:"created_at,omitempty"`
	UpdatedAt            string   `json:"updated_at,omitempty"`
}

type ModelListResult struct {
	Items    []ModelSummary `json:"items"`
	Total    int64          `json:"total"`
	Page     int            `json:"page"`
	PageSize int            `json:"page_size"`
}

type QuestionnaireBindingResult struct {
	QuestionnaireCode    string `json:"questionnaire_code"`
	QuestionnaireVersion string `json:"questionnaire_version"`
	Title                string `json:"title,omitempty"`
	QuestionCount        int    `json:"question_count"`
}

type OptionsResult struct {
	Kinds      []Option `json:"kinds"`
	Categories []Option `json:"categories"`
	Algorithms []Option `json:"algorithms"`
	SubKinds   []Option `json:"sub_kinds"`
	Tags       []Option `json:"tags,omitempty"`
}

type ValidationResult struct {
	Valid  bool     `json:"valid"`
	Errors []string `json:"errors"`
}

type PreviewReportResult struct {
	Message string `json:"message"`
}
