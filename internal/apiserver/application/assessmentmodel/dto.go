package assessmentmodel

import (
	"encoding/json"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/report"
)

const (
	KindPersonality                    = "personality"
	KindBehaviorAbility                = "behavior_ability"
	KindMedicalScale                   = "medical_scale"
	KindCognitive                      = "cognitive"
	KindCustom                         = "custom"
	SubKindTypology                    = "typology"
	SubKindScale                       = "scale"
	StatusDraft                        = "draft"
	StatusPublished                    = "published"
	StatusArchived                     = "archived"
	PayloadFormatScaleV1               = "assessmentmodel.behavior_ability.scale.v1"
	PayloadFormatMedicalScaleV1        = "assessmentmodel.medical_scale.scale.v1"
	PayloadFormatPersonalityTypologyV1 = "assessmentmodel.personality.typology.v1"
)

type Option struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

type ListModelsDTO struct {
	Kind      string
	SubKind   string
	Status    string
	Keyword   string
	Category  string
	Algorithm string
	Page      int
	PageSize  int
}

type CreateModelDTO struct {
	Code                 string
	Kind                 string
	SubKind              string
	Algorithm            string
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
	SubKind     string
	Algorithm   string
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
	SubKind              string   `json:"sub_kind,omitempty"`
	Algorithm            string   `json:"algorithm,omitempty"`
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

type ValidationIssue struct {
	Field   string `json:"field"`
	Message string `json:"message"`
	Code    string `json:"code,omitempty"`
	Level   string `json:"level,omitempty"`
}

type ValidationResult struct {
	Passed bool              `json:"passed"`
	Valid  bool              `json:"valid"` // deprecated: mirror Passed for backward compatibility
	Issues []ValidationIssue `json:"issues"`
	Errors []string          `json:"errors"` // deprecated: derived from Issues for backward compatibility
}

func NewValidationResult(issues []ValidationIssue) *ValidationResult {
	passed := len(issues) == 0
	result := &ValidationResult{
		Passed: passed,
		Valid:  passed,
		Issues: issues,
	}
	if len(issues) > 0 {
		result.Errors = make([]string, 0, len(issues))
		for _, issue := range issues {
			result.Errors = append(result.Errors, issue.Message)
		}
	}
	return result
}

type PreviewOutcome struct {
	Code  string `json:"code,omitempty"`
	Title string `json:"title,omitempty"`
}

type PreviewReportResult struct {
	Outcome PreviewOutcome          `json:"outcome"`
	Scores  map[string]float64      `json:"scores,omitempty"`
	Report  *report.InterpretReport `json:"report"`
}
