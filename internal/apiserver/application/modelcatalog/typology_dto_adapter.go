package modelcatalog

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/typology"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func typologyListInput(dto ListModelsDTO) typology.ListInput {
	return typology.ListInput{
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

func typologyCreateInput(dto CreateModelDTO) typology.CreateInput {
	return typology.CreateInput{
		Code:                 dto.Code,
		Title:                dto.Title,
		Description:          dto.Description,
		SubKind:              dto.SubKind,
		Algorithm:            dto.Algorithm,
		ProductChannel:       dto.ProductChannel,
		Category:             dto.Category,
		Tags:                 dto.Tags,
		QuestionnaireCode:    dto.QuestionnaireCode,
		QuestionnaireVersion: dto.QuestionnaireVersion,
	}
}

func typologyUpdateBasicInfoInput(dto UpdateBasicInfoDTO) typology.UpdateBasicInfoInput {
	return typology.UpdateBasicInfoInput{
		Code:           dto.Code,
		Title:          dto.Title,
		Description:    dto.Description,
		SubKind:        dto.SubKind,
		Algorithm:      dto.Algorithm,
		ProductChannel: dto.ProductChannel,
		Category:       dto.Category,
		Tags:           dto.Tags,
	}
}

func typologyBindInput(dto BindQuestionnaireDTO) typology.BindQuestionnaireInput {
	return typology.BindQuestionnaireInput{
		Code:                 dto.Code,
		QuestionnaireCode:    dto.QuestionnaireCode,
		QuestionnaireVersion: dto.QuestionnaireVersion,
	}
}

func typologyDefinitionInput(dto DefinitionDTO) typology.DefinitionInput {
	return typology.DefinitionInput{
		SubKind:       dto.SubKind,
		Algorithm:     dto.Algorithm,
		PayloadFormat: dto.PayloadFormat,
		Payload:       dto.Payload,
	}
}

func summaryFromTypology(result *typology.ModelSummary) *ModelSummary {
	if result == nil {
		return nil
	}
	summary := &ModelSummary{
		Code:                 result.Code,
		Kind:                 result.Kind,
		SubKind:              result.SubKind,
		Algorithm:            result.Algorithm,
		ProductChannel:       result.ProductChannel,
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
	populateModelSummaryIdentity(summary, domain.KindPersonality, domain.SubKind(result.SubKind), domain.Algorithm(result.Algorithm), domain.ProductChannel(result.ProductChannel))
	return summary
}

func definitionFromTypology(result *typology.DefinitionResult) *DefinitionDTO {
	if result == nil {
		return nil
	}
	dto := &DefinitionDTO{
		Kind:           result.Kind,
		SubKind:        result.SubKind,
		Algorithm:      result.Algorithm,
		ProductChannel: result.ProductChannel,
		PayloadFormat:  result.PayloadFormat,
		Payload:        result.Payload,
	}
	populateDefinitionIdentity(dto, domain.KindPersonality, domain.SubKind(result.SubKind), domain.Algorithm(result.Algorithm), domain.ProductChannel(result.ProductChannel))
	return dto
}

func validationFailedFromTypologyIssues(issues []typology.ValidationIssue) error {
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

func validationFromTypology(result *typology.ValidationResult) *ValidationResult {
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

func questionnaireFromTypology(result *typology.QuestionnaireBindingResult) *QuestionnaireBindingResult {
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

func summariesFromTypologyList(result *typology.ModelListResult) []ModelSummary {
	if result == nil {
		return nil
	}
	items := make([]ModelSummary, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, *summaryFromTypology(&item))
	}
	return items
}
