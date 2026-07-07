package modelcatalog

import (
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"go.mongodb.org/mongo-driver/bson"
)

type Mapper struct{}

func NewMapper() *Mapper {
	return &Mapper{}
}

func (Mapper) ToPO(snapshot *domain.PublishedModelSnapshot) *PublishedAssessmentModelPO {
	if snapshot == nil {
		return nil
	}
	status := snapshot.Model.Status
	if status == "" {
		status = statusPublished
	}
	source := bson.M{}
	for key, value := range snapshot.Source {
		source[key] = value
	}
	schemaVersion := snapshot.SchemaVersion
	if schemaVersion == "" {
		schemaVersion = domain.SchemaVersionV2
	}
	return &PublishedAssessmentModelPO{
		SchemaVersion:        schemaVersion,
		PayloadFormat:        snapshot.PayloadFormat,
		ModelProductChannel:  string(domain.ResolveProductChannel(snapshot.Model.Kind, snapshot.Model.ProductChannel)),
		ModelKind:            string(snapshot.Model.Kind),
		ModelSubKind:         string(snapshot.Model.SubKind),
		ModelAlgorithm:       string(snapshot.Model.Algorithm),
		ModelCode:            snapshot.Model.Code,
		ModelVersion:         snapshot.Model.Version,
		Title:                snapshot.Model.Title,
		Status:               status,
		DecisionKind:         string(snapshot.Decision.Kind),
		QuestionnaireCode:    snapshot.Binding.QuestionnaireCode,
		QuestionnaireVersion: snapshot.Binding.QuestionnaireVersion,
		Source:               source,
		Payload:              append([]byte(nil), snapshot.Payload...),
	}
}

func (Mapper) ToPublished(po *PublishedAssessmentModelPO) *domain.PublishedModelSnapshot {
	if po == nil {
		return nil
	}
	source := make(domain.SourceRef, len(po.Source))
	for key, value := range po.Source {
		source[key] = value
	}
	kind := domain.Kind(po.ModelKind)
	productChannel := domain.ProductChannel(po.ModelProductChannel)
	if productChannel == "" {
		productChannel = domain.DefaultProductChannelFor(kind)
	}
	return &domain.PublishedModelSnapshot{
		SchemaVersion: po.SchemaVersion,
		PayloadFormat: po.PayloadFormat,
		Model: domain.ModelDefinition{
			ProductChannel: productChannel,
			Kind:           kind,
			SubKind:        domain.SubKind(po.ModelSubKind),
			Algorithm:      domain.Algorithm(po.ModelAlgorithm),
			Code:           po.ModelCode,
			Version:        po.ModelVersion,
			Title:          po.Title,
			Status:         po.Status,
		},
		Binding: domain.QuestionnaireBinding{
			QuestionnaireCode:    po.QuestionnaireCode,
			QuestionnaireVersion: po.QuestionnaireVersion,
		},
		Decision: domain.DecisionSpec{
			Kind: domain.DecisionKind(po.DecisionKind),
		},
		Source:  source,
		Payload: append([]byte(nil), po.Payload...),
	}
}

func (m Mapper) ToLegacySnapshot(po *PublishedAssessmentModelPO) *domain.Snapshot {
	return domain.LegacyFromPublished(m.ToPublished(po))
}
