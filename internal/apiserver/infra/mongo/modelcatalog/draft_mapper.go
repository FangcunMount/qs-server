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
		ProductChannel:          string(domain.NormalizeProductChannel(domain.ResolveProductChannel(model.Kind, model.ProductChannel))),
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
	kind := domain.Kind(po.Kind)
	productChannel := domain.NormalizeProductChannel(domain.ProductChannel(po.ProductChannel))
	if productChannel == "" {
		productChannel = domain.DefaultProductChannelFor(kind)
	}
	return &domain.AssessmentModel{
		ID:             po.ID.Hex(),
		Code:           po.Code,
		Kind:           kind,
		SubKind:        domain.SubKind(po.SubKind),
		Algorithm:      domain.Algorithm(po.Algorithm),
		ProductChannel: productChannel,
		Title:          po.Title,
		Description:    po.Description,
		Category:       po.Category,
		Tags:           append([]string(nil), po.Tags...),
		Status:         domain.ModelStatus(po.Status),
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
