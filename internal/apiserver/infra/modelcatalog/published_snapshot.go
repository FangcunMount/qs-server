package modelcatalog

import (
	"encoding/json"
	"fmt"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/publishing"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scoring/snapshot"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/typology"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

func BuildScalePublishedSnapshot(model *scalesnapshot.ScaleSnapshot) (*domain.PublishedModelSnapshot, error) {
	return publishing.BuildScoringPublishedSnapshotFromScale(model)
}

func BuildMBTIPublishedSnapshot(model *modeltypology.MBTILegacyModel) (*domain.PublishedModelSnapshot, error) {
	payload, format, err := encodeTypologyPayload(modeltypology.FromMBTI(model))
	if err != nil {
		return nil, err
	}
	status := model.Status
	if status == "" {
		status = "published"
	}
	return &domain.PublishedModelSnapshot{
		SchemaVersion: domain.SchemaVersionV2,
		PayloadFormat: format,
		Model: domain.ModelDefinition{
			ProductChannel: domain.ProductChannelTypology,
			Kind:           domain.KindTypology,
			SubKind:        domain.SubKindTypology,
			Algorithm:      domain.AlgorithmMBTI,
			Code:           model.Code,
			Version:        model.Version,
			Title:          model.Title,
			Status:         status,
		},
		Binding: domain.QuestionnaireBinding{
			QuestionnaireCode:    model.QuestionnaireCode,
			QuestionnaireVersion: model.QuestionnaireVersion,
		},
		Decision: domain.DecisionSpec{Kind: domain.DecisionKindPoleComposition},
		Source: domain.SourceRef{
			"questions_repo": model.Source.QuestionsRepo,
			"source_site":    model.Source.SourceSite,
			"license":        model.Source.License,
			"attribution":    model.Source.Attribution,
			"non_commercial": model.Source.NonCommercial,
		},
		Payload: payload,
	}, nil
}

func BuildSBTIPublishedSnapshot(model *modeltypology.SBTILegacyModel) (*domain.PublishedModelSnapshot, error) {
	payload, format, err := encodeTypologyPayload(modeltypology.FromSBTI(model))
	if err != nil {
		return nil, err
	}
	status := model.Status
	if status == "" {
		status = "published"
	}
	return &domain.PublishedModelSnapshot{
		SchemaVersion: domain.SchemaVersionV2,
		PayloadFormat: format,
		Payload:       payload,
		Model: domain.ModelDefinition{
			ProductChannel: domain.ProductChannelTypology,
			Kind:           domain.KindTypology,
			SubKind:        domain.SubKindTypology,
			Algorithm:      domain.AlgorithmSBTI,
			Code:           model.Code,
			Version:        model.Version,
			Title:          model.Title,
			Status:         status,
		},
		Binding: domain.QuestionnaireBinding{
			QuestionnaireCode:    model.QuestionnaireCode,
			QuestionnaireVersion: model.QuestionnaireVersion,
		},
		Decision: domain.DecisionSpec{Kind: domain.DecisionKindNearestPattern},
		Source: domain.SourceRef{
			"wiki_repo":      model.Source.WikiRepo,
			"source_site":    model.Source.SourceSite,
			"license":        model.Source.License,
			"attribution":    model.Source.Attribution,
			"non_commercial": model.Source.NonCommercial,
			"image_base_url": model.Source.ImageBaseURL,
		},
	}, nil
}

func RefFromPublished(snapshot *domain.PublishedModelSnapshot) port.Ref {
	if snapshot == nil {
		return port.Ref{}
	}
	return port.Ref{
		Kind:      snapshot.Model.Kind,
		SubKind:   snapshot.Model.SubKind,
		Algorithm: snapshot.Model.Algorithm,
		Code:      snapshot.Model.Code,
		Version:   snapshot.Model.Version,
		Title:     snapshot.Model.Title,
	}
}

func RefMatchesPublished(ref port.Ref, snapshot *domain.PublishedModelSnapshot) bool {
	if snapshot == nil || ref.Code == "" || ref.Version == "" {
		return false
	}
	got := RefFromPublished(snapshot)
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
