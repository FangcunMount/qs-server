package personality

import "encoding/json"

const (
	KindPersonality         = "personality"
	SubKindTypology         = "typology"
	StatusDraft             = "draft"
	StatusPublished         = "published"
	StatusArchived          = "archived"
	PayloadFormatTypologyV1 = "assessmentmodel.personality.typology.v1"
)

type ListInput struct {
	Kind      string
	SubKind   string
	Status    string
	Keyword   string
	Category  string
	Algorithm string
	Page      int
	PageSize  int
}

type CreateInput struct {
	Code                 string
	Title                string
	Description          string
	SubKind              string
	Algorithm            string
	Category             string
	Tags                 []string
	QuestionnaireCode    string
	QuestionnaireVersion string
}

type UpdateBasicInfoInput struct {
	Code        string
	Title       string
	Description string
	SubKind     string
	Algorithm   string
	Category    string
	Tags        []string
}

type BindQuestionnaireInput struct {
	Code                 string
	QuestionnaireCode    string
	QuestionnaireVersion string
}

type DefinitionInput struct {
	SubKind       string
	Algorithm     string
	PayloadFormat string
	Payload       json.RawMessage
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

type DefinitionResult struct {
	Kind          string          `json:"kind"`
	SubKind       string          `json:"sub_kind,omitempty"`
	Algorithm     string          `json:"algorithm,omitempty"`
	PayloadFormat string          `json:"payload_format"`
	Payload       json.RawMessage `json:"payload"`
}

type ValidationIssue struct {
	Field   string `json:"field"`
	Message string `json:"message"`
	Code    string `json:"code,omitempty"`
	Level   string `json:"level,omitempty"`
}

type ValidationResult struct {
	Passed bool              `json:"passed"`
	Valid  bool              `json:"valid"`
	Issues []ValidationIssue `json:"issues"`
	Errors []string          `json:"errors"`
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
