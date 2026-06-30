package personality

import (
	"encoding/json"
	"time"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	personalitydomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/personality"
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
		Kind:          KindPersonality,
		SubKind:       string(model.SubKind),
		Algorithm:     string(model.Algorithm),
		PayloadFormat: model.Definition.Format,
		Payload:       normalizeDefinitionPayloadForAPI(model),
	}
}

// normalizeDefinitionPayloadForAPI returns RuntimeSpec JSON for the operating editor.
// Draft definitions may store either RuntimeSpec or the full typology Payload envelope.
func normalizeDefinitionPayloadForAPI(model *domain.AssessmentModel) []byte {
	raw := append([]byte(nil), model.Definition.Data...)
	if len(raw) == 0 {
		return raw
	}
	_, runtime, err := personalitydomain.PayloadAndRuntimeSpecFromModel(model)
	if err != nil || runtime == nil {
		return raw
	}
	data, err := json.Marshal(runtime)
	if err != nil {
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
