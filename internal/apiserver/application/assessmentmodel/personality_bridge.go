package assessmentmodel

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/application/assessmentmodel/personality"
)

func personalityListInput(dto ListModelsDTO) personality.ListInput {
	return personality.ListInput{
		Kind:      dto.Kind,
		SubKind:   dto.SubKind,
		Status:    dto.Status,
		Keyword:   dto.Keyword,
		Category:  dto.Category,
		Algorithm: dto.Algorithm,
		Page:      dto.Page,
		PageSize:  dto.PageSize,
	}
}

func personalityCreateInput(dto CreateModelDTO) personality.CreateInput {
	return personality.CreateInput{
		Code:                 dto.Code,
		Title:                dto.Title,
		Description:          dto.Description,
		SubKind:              dto.SubKind,
		Algorithm:            dto.Algorithm,
		Category:             dto.Category,
		Tags:                 dto.Tags,
		QuestionnaireCode:    dto.QuestionnaireCode,
		QuestionnaireVersion: dto.QuestionnaireVersion,
	}
}

func personalityUpdateBasicInfoInput(dto UpdateBasicInfoDTO) personality.UpdateBasicInfoInput {
	return personality.UpdateBasicInfoInput{
		Code:        dto.Code,
		Title:       dto.Title,
		Description: dto.Description,
		SubKind:     dto.SubKind,
		Algorithm:   dto.Algorithm,
		Category:    dto.Category,
		Tags:        dto.Tags,
	}
}

func personalityBindInput(dto BindQuestionnaireDTO) personality.BindQuestionnaireInput {
	return personality.BindQuestionnaireInput{
		Code:                 dto.Code,
		QuestionnaireCode:    dto.QuestionnaireCode,
		QuestionnaireVersion: dto.QuestionnaireVersion,
	}
}

func personalityDefinitionInput(dto DefinitionDTO) personality.DefinitionInput {
	return personality.DefinitionInput{
		SubKind:       dto.SubKind,
		Algorithm:     dto.Algorithm,
		PayloadFormat: dto.PayloadFormat,
		Payload:       dto.Payload,
	}
}

func summaryFromPersonality(result *personality.ModelSummary) *ModelSummary {
	if result == nil {
		return nil
	}
	return &ModelSummary{
		Code:                 result.Code,
		Kind:                 result.Kind,
		SubKind:              result.SubKind,
		Algorithm:            result.Algorithm,
		Title:                result.Title,
		Description:          result.Description,
		Status:               result.Status,
		Category:             result.Category,
		Tags:                 append([]string(nil), result.Tags...),
		QuestionnaireCode:    result.QuestionnaireCode,
		QuestionnaireVersion: result.QuestionnaireVersion,
		CreatedAt:            result.CreatedAt,
		UpdatedAt:            result.UpdatedAt,
	}
}

func definitionFromPersonality(result *personality.DefinitionResult) *DefinitionDTO {
	if result == nil {
		return nil
	}
	return &DefinitionDTO{
		Kind:          result.Kind,
		SubKind:       result.SubKind,
		Algorithm:     result.Algorithm,
		PayloadFormat: result.PayloadFormat,
		Payload:       result.Payload,
	}
}

func validationFailedFromPersonalityIssues(issues []personality.ValidationIssue) error {
	mapped := make([]ValidationIssue, 0, len(issues))
	for _, issue := range issues {
		mapped = append(mapped, ValidationIssue{
			Field:   issue.Field,
			Message: issue.Message,
			Code:    issue.Code,
			Level:   issue.Level,
		})
	}
	return NewValidationFailedError(mapped)
}

func validationFromPersonality(result *personality.ValidationResult) *ValidationResult {
	if result == nil {
		return NewValidationResult(nil)
	}
	issues := make([]ValidationIssue, 0, len(result.Issues))
	for _, issue := range result.Issues {
		issues = append(issues, ValidationIssue{
			Field:   issue.Field,
			Message: issue.Message,
			Code:    issue.Code,
			Level:   issue.Level,
		})
	}
	return NewValidationResult(issues)
}

func questionnaireFromPersonality(result *personality.QuestionnaireBindingResult) *QuestionnaireBindingResult {
	if result == nil {
		return nil
	}
	return &QuestionnaireBindingResult{
		QuestionnaireCode:    result.QuestionnaireCode,
		QuestionnaireVersion: result.QuestionnaireVersion,
		Title:                result.Title,
		QuestionCount:        result.QuestionCount,
	}
}

func summariesFromPersonalityList(result *personality.ModelListResult) []ModelSummary {
	if result == nil {
		return nil
	}
	items := make([]ModelSummary, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, *summaryFromPersonality(&item))
	}
	return items
}
