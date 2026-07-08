package personality

import (
	"time"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/publishing"
)

func summaryFromModel(model *domain.AssessmentModel) *ModelSummary {
	if model == nil {
		return nil
	}
	return &ModelSummary{
		Code:                 model.Code,
		Kind:                 KindPersonality,
		SubKind:              string(model.SubKind),
		Algorithm:            string(model.Algorithm),
		ProductChannel:       string(domain.ResolveProductChannel(model.Kind, model.ProductChannel)),
		Title:                model.Title,
		Description:          model.Description,
		Status:               string(model.Status),
		Category:             model.Category,
		Tags:                 append([]string(nil), model.Tags...),
		QuestionnaireCode:    model.Binding.QuestionnaireCode,
		QuestionnaireVersion: model.Binding.QuestionnaireVersion,
		CreatedAt:            model.CreatedAt.Format(time.DateTime),
		UpdatedAt:            model.UpdatedAt.Format(time.DateTime),
	}
}

func definitionFromModel(model *domain.AssessmentModel) *DefinitionResult {
	if model == nil {
		return nil
	}
	return &DefinitionResult{
		Kind:           KindPersonality,
		SubKind:        string(model.SubKind),
		Algorithm:      string(model.Algorithm),
		ProductChannel: string(domain.ResolveProductChannel(model.Kind, model.ProductChannel)),
		PayloadFormat:  model.Definition.Format,
		Payload:        normalizeDefinitionPayloadForAPI(model),
	}
}

// 正态izeDefinition载荷ForAPI returns operating editor 载荷 结构。
func normalizeDefinitionPayloadForAPI(model *domain.AssessmentModel) []byte {
	raw := append([]byte(nil), model.Definition.Data...)
	if len(raw) == 0 {
		return raw
	}
	payload, runtime, err := publishing.PersonalityPayloadAndRuntimeSpecFromModel(model)
	if err != nil || runtime == nil {
		return raw
	}
	data, err := buildEditorDefinitionPayload(model, payload, runtime)
	if err != nil || len(data) == 0 {
		return raw
	}
	return data
}

func normalizeCreateInput(input CreateInput) (domain.SubKind, domain.Algorithm, error) {
	subKind := domain.SubKind(input.SubKind)
	if subKind == "" {
		subKind = domain.SubKindTypology
	}
	algorithm := domain.Algorithm(input.Algorithm)
	if algorithm == "" {
		return subKind, "", domain.ErrInvalidArgument
	}
	return subKind, algorithm, nil
}

func domainIssuesToValidation(issues []domain.DomainValidationIssue) []ValidationIssue {
	if len(issues) == 0 {
		return nil
	}
	out := make([]ValidationIssue, 0, len(issues))
	for _, issue := range issues {
		out = append(out, ValidationIssue{
			Field:   issue.Field,
			Message: issue.Message,
			Code:    issue.Code,
			Level:   string(issue.Level),
		})
	}
	return out
}
