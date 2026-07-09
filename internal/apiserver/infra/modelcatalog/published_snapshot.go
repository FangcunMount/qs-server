package modelcatalog

import (
	"encoding/json"
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/publishedmodel"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/scale"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/typology"
)

func BuildScalePublishedSnapshot(model *scalesnapshot.ScaleSnapshot) (*port.PublishedModel, error) {
	return publishedmodel.BuildAssessmentSnapshotFromScale(model)
}

func BuildMBTIPublishedSnapshot(model *modeltypology.MBTILegacyModel) (*port.PublishedModel, error) {
	payload, format, err := encodeTypologyPayload(modeltypology.FromMBTI(model))
	if err != nil {
		return nil, err
	}
	status := model.Status
	if status == "" {
		status = "published"
	}
	return &port.PublishedModel{
		SchemaVersion:        domain.SchemaVersionV2,
		PayloadFormat:        format,
		ProductChannel:       domain.ProductChannelTypology,
		Kind:                 domain.KindTypology,
		SubKind:              domain.SubKindTypology,
		Algorithm:            domain.AlgorithmMBTI,
		Code:                 model.Code,
		Version:              model.Version,
		Title:                model.Title,
		Status:               status,
		DecisionKind:         domain.DecisionKindPoleComposition,
		QuestionnaireCode:    model.QuestionnaireCode,
		QuestionnaireVersion: model.QuestionnaireVersion,
		Source: map[string]any{
			"questions_repo": model.Source.QuestionsRepo,
			"source_site":    model.Source.SourceSite,
			"license":        model.Source.License,
			"attribution":    model.Source.Attribution,
			"non_commercial": model.Source.NonCommercial,
		},
		Payload: payload,
	}, nil
}

func BuildSBTIPublishedSnapshot(model *modeltypology.SBTILegacyModel) (*port.PublishedModel, error) {
	payload, format, err := encodeTypologyPayload(modeltypology.FromSBTI(model))
	if err != nil {
		return nil, err
	}
	status := model.Status
	if status == "" {
		status = "published"
	}
	return &port.PublishedModel{
		SchemaVersion:        domain.SchemaVersionV2,
		PayloadFormat:        format,
		Payload:              payload,
		ProductChannel:       domain.ProductChannelTypology,
		Kind:                 domain.KindTypology,
		SubKind:              domain.SubKindTypology,
		Algorithm:            domain.AlgorithmSBTI,
		Code:                 model.Code,
		Version:              model.Version,
		Title:                model.Title,
		Status:               status,
		DecisionKind:         domain.DecisionKindNearestPattern,
		QuestionnaireCode:    model.QuestionnaireCode,
		QuestionnaireVersion: model.QuestionnaireVersion,
		Source: map[string]any{
			"wiki_repo":      model.Source.WikiRepo,
			"source_site":    model.Source.SourceSite,
			"license":        model.Source.License,
			"attribution":    model.Source.Attribution,
			"non_commercial": model.Source.NonCommercial,
			"image_base_url": model.Source.ImageBaseURL,
		},
	}, nil
}

func RefFromPublished(model *port.PublishedModel) port.Ref {
	if model == nil {
		return port.Ref{}
	}
	return port.Ref{
		Kind:      model.Kind,
		SubKind:   model.SubKind,
		Algorithm: model.Algorithm,
		Code:      model.Code,
		Version:   model.Version,
		Title:     model.Title,
	}
}

func RefMatchesPublished(ref port.Ref, model *port.PublishedModel) bool {
	if model == nil || ref.Code == "" || ref.Version == "" {
		return false
	}
	got := RefFromPublished(model)
	return ref.Kind == got.Kind &&
		ref.SubKind == got.SubKind &&
		ref.Algorithm == got.Algorithm &&
		ref.Code == got.Code &&
		ref.Version == got.Version
}

func encodeTypologyPayload(model *modeltypology.Payload) ([]byte, string, error) {
	if model == nil {
		return nil, "", fmt.Errorf("typology model is nil")
	}
	payload, err := json.Marshal(model)
	if err != nil {
		return nil, "", fmt.Errorf("marshal typology payload: %w", err)
	}
	return payload, domain.PayloadFormatPersonalityTypologyV1, nil
}
