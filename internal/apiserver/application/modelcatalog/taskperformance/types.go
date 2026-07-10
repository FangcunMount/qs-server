package taskperformance

import (
	"encoding/json"
	"time"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

const KindCognitive = "cognitive"

type ListInput struct {
	Status   string
	Keyword  string
	Page     int
	PageSize int
}

type CreateInput struct {
	Code                 string
	Title                string
	Description          string
	ProductChannel       string
	Category             string
	Tags                 []string
	QuestionnaireCode    string
	QuestionnaireVersion string
}

type UpdateBasicInfoInput struct {
	Code           string
	Title          string
	Description    string
	ProductChannel string
	Category       string
	Tags           []string
}

type BindQuestionnaireInput struct {
	Code                 string
	QuestionnaireCode    string
	QuestionnaireVersion string
}

// DefinitionInput is the cognitive authoring command contract. API gateways
// import legacy payloads before invoking the command.
type DefinitionInput struct {
	Payload      json.RawMessage
	DefinitionV2 *domain.Definition
	Norms        []*domain.Norm
}

type ModelSummary struct {
	Code                 string   `json:"code"`
	Kind                 string   `json:"kind"`
	Algorithm            string   `json:"algorithm,omitempty"`
	ProductChannel       string   `json:"product_channel,omitempty"`
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
	Items      []ModelSummary `json:"items"`
	Total      int64          `json:"total"`
	Page       int            `json:"page"`
	PageSize   int            `json:"page_size"`
	TotalPages int            `json:"total_pages,omitempty"`
}

type DefinitionResult struct {
	Kind           string          `json:"kind"`
	Algorithm      string          `json:"algorithm,omitempty"`
	ProductChannel string          `json:"product_channel,omitempty"`
	PayloadFormat  string          `json:"payload_format"`
	Payload        json.RawMessage `json:"payload"`
}

type QuestionnaireBindingResult struct {
	QuestionnaireCode    string `json:"questionnaire_code"`
	QuestionnaireVersion string `json:"questionnaire_version"`
}

func summaryFromModel(model *domain.AssessmentModel) *ModelSummary {
	if model == nil {
		return nil
	}
	return &ModelSummary{
		Code:                 model.Code,
		Kind:                 KindCognitive,
		Algorithm:            string(model.Algorithm),
		ProductChannel:       string(domain.ResolveProductChannel(model.Kind, model.ProductChannel)),
		Title:                model.Title,
		Description:          model.Description,
		Status:               string(model.Status),
		Category:             model.Category,
		Tags:                 append([]string(nil), model.Tags...),
		QuestionnaireCode:    model.Binding.QuestionnaireCode,
		QuestionnaireVersion: model.Binding.QuestionnaireVersion,
		CreatedAt:            model.CreatedAt.Format(time.RFC3339),
		UpdatedAt:            model.UpdatedAt.Format(time.RFC3339),
	}
}
