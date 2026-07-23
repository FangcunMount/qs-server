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
	return &PublishedAssessmentModelPO{
		SchemaVersion:           schemaVersion,
		RecordRole:              recordRolePublishedSnapshot,
		ReleaseStatus:           string(domain.NormalizeReleaseStatus(model.ReleaseStatus)),
		Kind:                    string(model.Kind),
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
		DefinitionSchemaVersion: definitionSchemaVersion(model.DefinitionV2),
		DefinitionV2:            definitionToPO(model.DefinitionV2),
		PublishedAt:             model.PublishedAt,
		ReleaseArchivedAt:       model.ReleaseArchivedAt,
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
	decisionKind := domain.DecisionKind(po.DecisionKind)
	subKind := domain.CanonicalSubKindFor(kind)
	algorithmFamily, ok := domain.AlgorithmFamilyFromDecisionKind(decisionKind)
	if !ok {
		algorithmFamily, _ = domain.AlgorithmFamilyFromIdentity(kind, subKind, domain.Algorithm(po.Algorithm))
	}
	model := &port.PublishedModel{
		SchemaVersion:        po.SchemaVersion,
		ProductChannel:       domain.DefaultProductChannelFor(kind),
		Kind:                 kind,
		SubKind:              subKind,
		Algorithm:            domain.Algorithm(po.Algorithm),
		AlgorithmFamily:      algorithmFamily,
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
		ReleaseStatus:        domain.NormalizeReleaseStatus(domain.ReleaseStatus(po.ReleaseStatus)),
		PublishedAt:          po.PublishedAt,
		ReleaseArchivedAt:    po.ReleaseArchivedAt,
		DecisionKind:         decisionKind,
		QuestionnaireCode:    po.QuestionnaireCode,
		QuestionnaireVersion: po.QuestionnaireVersion,
		Source:               source,
		DefinitionV2:         definitionFromPO(po.DefinitionV2),
	}
	return model
}
