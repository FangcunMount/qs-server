package modelcatalog

import (
	"time"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// ModelSummaryFromAssessmentModel projects aggregate metadata shared by
// management, query and publication use cases.
func ModelSummaryFromAssessmentModel(model *domain.AssessmentModel) *ModelSummary {
	if model == nil {
		return nil
	}
	result := &ModelSummary{
		Code:                 model.Code,
		Kind:                 DomainKindToAPIKind(model.Kind),
		SubKind:              string(model.SubKind),
		Algorithm:            string(model.Algorithm),
		Title:                model.Title,
		Description:          model.Description,
		Status:               string(model.Status),
		Category:             model.Category,
		Stages:               append([]string(nil), model.Stages...),
		ApplicableAges:       append([]string(nil), model.ApplicableAges...),
		Reporters:            append([]string(nil), model.Reporters...),
		Tags:                 append([]string(nil), model.Tags...),
		QuestionnaireCode:    model.Binding.QuestionnaireCode,
		QuestionnaireVersion: model.Binding.QuestionnaireVersion,
		CreatedAt:            model.CreatedAt.Format(time.RFC3339),
		UpdatedAt:            model.UpdatedAt.Format(time.RFC3339),
	}
	PopulateModelSummaryIdentity(result, model.Kind, model.SubKind, model.Algorithm, model.ProductChannel)
	return result
}
