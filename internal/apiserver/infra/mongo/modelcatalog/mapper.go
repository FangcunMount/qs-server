package modelcatalog

import (
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"go.mongodb.org/mongo-driver/bson"
)

type Mapper struct{}

func NewMapper() *Mapper {
	return &Mapper{}
}

func (Mapper) ToPO(model *port.PublishedModel) *PublishedAssessmentModelPO {
	if model == nil {
		return nil
	}
	status := model.Status
	if status == "" {
		status = statusPublished
	}
	source := bson.M{}
	for key, value := range model.Source {
		source[key] = value
	}
	schemaVersion := model.SchemaVersion
	if schemaVersion == "" {
		schemaVersion = domain.SchemaVersionV2
	}
	kind := model.Kind
	productChannel := domain.ResolveProductChannel(kind, model.ProductChannel)
	return &PublishedAssessmentModelPO{
		SchemaVersion:           schemaVersion,
		PayloadFormat:           model.PayloadFormat,
		RecordRole:              recordRolePublishedSnapshot,
		IsActivePublished:       true,
		ProductChannel:          string(productChannel),
		Kind:                    string(kind),
		SubKind:                 string(model.SubKind),
		Algorithm:               string(model.Algorithm),
		Code:                    model.Code,
		ReleaseVersion:          model.Version,
		Title:                   model.Title,
		Description:             model.Description,
		Category:                model.Category,
		Stages:                  append([]string(nil), model.Stages...),
		ApplicableAges:          append([]string(nil), model.ApplicableAges...),
		Reporters:               append([]string(nil), model.Reporters...),
		Tags:                    append([]string(nil), model.Tags...),
		Status:                  status,
		DecisionKind:            string(model.DecisionKind),
		QuestionnaireCode:       model.QuestionnaireCode,
		QuestionnaireVersion:    model.QuestionnaireVersion,
		Source:                  source,
		Payload:                 append([]byte(nil), model.Payload...),
		DefinitionSchemaVersion: definitionSchemaVersion(model.DefinitionV2),
		DefinitionV2:            definitionToPO(model.DefinitionV2),
	}
}

func (Mapper) ToPublished(po *PublishedAssessmentModelPO) *port.PublishedModel {
	if po == nil {
		return nil
	}
	source := make(map[string]any, len(po.Source))
	for key, value := range po.Source {
		source[key] = value
	}
	kind := domain.Kind(po.Kind)
	productChannel := domain.ProductChannel(po.ProductChannel)
	if productChannel == "" {
		productChannel = domain.DefaultProductChannelFor(kind)
	}
	return &port.PublishedModel{
		SchemaVersion:        po.SchemaVersion,
		PayloadFormat:        po.PayloadFormat,
		ProductChannel:       productChannel,
		Kind:                 kind,
		SubKind:              domain.SubKind(po.SubKind),
		Algorithm:            domain.Algorithm(po.Algorithm),
		Code:                 po.Code,
		Version:              po.ReleaseVersion,
		Title:                po.Title,
		Description:          po.Description,
		Category:             po.Category,
		Stages:               append([]string(nil), po.Stages...),
		ApplicableAges:       append([]string(nil), po.ApplicableAges...),
		Reporters:            append([]string(nil), po.Reporters...),
		Tags:                 append([]string(nil), po.Tags...),
		Status:               po.Status,
		DecisionKind:         domain.DecisionKind(po.DecisionKind),
		QuestionnaireCode:    po.QuestionnaireCode,
		QuestionnaireVersion: po.QuestionnaireVersion,
		Source:               source,
		Payload:              append([]byte(nil), po.Payload...),
		DefinitionV2:         definitionFromPO(po.DefinitionV2),
	}
}
