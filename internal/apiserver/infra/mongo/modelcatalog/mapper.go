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
	kind := domain.NormalizeKind(model.Kind)
	productChannel := domain.NormalizeProductChannel(domain.ResolveProductChannel(kind, model.ProductChannel))
	return &PublishedAssessmentModelPO{
		SchemaVersion:        schemaVersion,
		PayloadFormat:        model.PayloadFormat,
		ModelProductChannel:  string(productChannel),
		ModelKind:            string(kind),
		ModelSubKind:         string(model.SubKind),
		ModelAlgorithm:       string(model.Algorithm),
		ModelCode:            model.Code,
		ModelVersion:         model.Version,
		Title:                model.Title,
		Status:               status,
		DecisionKind:         string(model.DecisionKind),
		QuestionnaireCode:    model.QuestionnaireCode,
		QuestionnaireVersion: model.QuestionnaireVersion,
		Source:               source,
		Payload:              append([]byte(nil), model.Payload...),
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
	kind := domain.NormalizeKind(domain.Kind(po.ModelKind))
	productChannel := domain.NormalizeProductChannel(domain.ProductChannel(po.ModelProductChannel))
	if productChannel == "" {
		productChannel = domain.DefaultProductChannelFor(kind)
	}
	return &port.PublishedModel{
		SchemaVersion:        po.SchemaVersion,
		PayloadFormat:        po.PayloadFormat,
		ProductChannel:       productChannel,
		Kind:                 kind,
		SubKind:              domain.SubKind(po.ModelSubKind),
		Algorithm:            domain.Algorithm(po.ModelAlgorithm),
		Code:                 po.ModelCode,
		Version:              po.ModelVersion,
		Title:                po.Title,
		Status:               po.Status,
		DecisionKind:         domain.DecisionKind(po.DecisionKind),
		QuestionnaireCode:    po.QuestionnaireCode,
		QuestionnaireVersion: po.QuestionnaireVersion,
		Source:               source,
		Payload:              append([]byte(nil), po.Payload...),
	}
}
