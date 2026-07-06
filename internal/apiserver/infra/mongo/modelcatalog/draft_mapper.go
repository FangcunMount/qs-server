package modelcatalog

import (
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

type DraftMapper struct{}

func NewDraftMapper() *DraftMapper {
	return &DraftMapper{}
}

func (DraftMapper) ToPO(model *domain.AssessmentModel) *AssessmentModelPO {
	if model == nil {
		return nil
	}
	return &AssessmentModelPO{
		Code:                    model.Code,
		Kind:                    string(model.Kind),
		SubKind:                 string(model.SubKind),
		Algorithm:               string(model.Algorithm),
		Title:                   model.Title,
		Description:             model.Description,
		Category:                model.Category,
		Tags:                    append([]string(nil), model.Tags...),
		Status:                  string(model.Status),
		QuestionnaireCode:       model.Binding.QuestionnaireCode,
		QuestionnaireVersion:    model.Binding.QuestionnaireVersion,
		DefinitionPayloadFormat: model.Definition.Format,
		DefinitionPayload:       append([]byte(nil), model.Definition.Data...),
		Version:                 model.Version,
		PublishedAt:             model.PublishedAt,
		ArchivedAt:              model.ArchivedAt,
	}
}

func (DraftMapper) ToDomain(po *AssessmentModelPO) *domain.AssessmentModel {
	if po == nil {
		return nil
	}
	return &domain.AssessmentModel{
		ID:          po.ID.Hex(),
		Code:        po.Code,
		Kind:        domain.Kind(po.Kind),
		SubKind:     domain.SubKind(po.SubKind),
		Algorithm:   domain.Algorithm(po.Algorithm),
		Title:       po.Title,
		Description: po.Description,
		Category:    po.Category,
		Tags:        append([]string(nil), po.Tags...),
		Status:      domain.ModelStatus(po.Status),
		Binding: domain.QuestionnaireBinding{
			QuestionnaireCode:    po.QuestionnaireCode,
			QuestionnaireVersion: po.QuestionnaireVersion,
		},
		Definition: domain.DefinitionPayload{
			Format: po.DefinitionPayloadFormat,
			Data:   append([]byte(nil), po.DefinitionPayload...),
		},
		Version:     po.Version,
		CreatedAt:   po.CreatedAt,
		UpdatedAt:   po.UpdatedAt,
		PublishedAt: po.PublishedAt,
		ArchivedAt:  po.ArchivedAt,
	}
}
