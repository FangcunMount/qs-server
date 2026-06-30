package assessmentmodel

import (
	"fmt"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/personality/typology"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/scale/snapshot"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/ruleset/codec"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/assessmentmodel"
)

func BuildScalePublishedSnapshot(model *scalesnapshot.ScaleSnapshot) (*domain.PublishedModelSnapshot, error) {
	if model == nil {
		return nil, fmt.Errorf("scale model is nil")
	}
	payload, format, err := codec.EncodeScale(model)
	if err != nil {
		return nil, err
	}
	status := model.Status
	if status == "" {
		status = "published"
	}
	version := model.ScaleVersion
	if version == "" {
		version = model.QuestionnaireVersion
	}
	return &domain.PublishedModelSnapshot{
		SchemaVersion: domain.SchemaVersionV2,
		PayloadFormat: format,
		Model: domain.ModelDefinition{
			Kind:      domain.KindScale,
			SubKind:   domain.SubKindEmpty,
			Algorithm: domain.AlgorithmScaleDefault,
			Code:      model.Code,
			Version:   version,
			Title:     model.Title,
			Status:    status,
		},
		Binding: domain.QuestionnaireBinding{
			QuestionnaireCode:    model.QuestionnaireCode,
			QuestionnaireVersion: model.QuestionnaireVersion,
		},
		Decision: domain.DecisionSpec{Kind: domain.DecisionKindScoreRange},
		Source:   domain.SourceRef{},
		Payload:  payload,
	}, nil
}

func BuildMBTIPublishedSnapshot(model *modeltypology.MBTILegacyModel) (*domain.PublishedModelSnapshot, error) {
	payload, format, err := codec.EncodeTypology(modeltypology.FromMBTI(model))
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
			Kind:      domain.KindPersonality,
			SubKind:   domain.SubKindTypology,
			Algorithm: domain.AlgorithmMBTI,
			Code:      model.Code,
			Version:   model.Version,
			Title:     model.Title,
			Status:    status,
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
	payload, format, err := codec.EncodeTypology(modeltypology.FromSBTI(model))
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
			Kind:      domain.KindPersonality,
			SubKind:   domain.SubKindTypology,
			Algorithm: domain.AlgorithmSBTI,
			Code:      model.Code,
			Version:   model.Version,
			Title:     model.Title,
			Status:    status,
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

func LegacySnapshotFromPublished(snapshot *domain.PublishedModelSnapshot) *domain.Snapshot {
	return domain.LegacyFromPublished(snapshot)
}

func LegacySnapshotFromScale(model *scalesnapshot.ScaleSnapshot) (*domain.Snapshot, error) {
	published, err := BuildScalePublishedSnapshot(model)
	if err != nil {
		return nil, err
	}
	return LegacySnapshotFromPublished(published), nil
}

func LegacySnapshotFromMBTI(model *modeltypology.MBTILegacyModel) (*domain.Snapshot, error) {
	published, err := BuildMBTIPublishedSnapshot(model)
	if err != nil {
		return nil, err
	}
	return LegacySnapshotFromPublished(published), nil
}

func LegacySnapshotFromSBTI(model *modeltypology.SBTILegacyModel) (*domain.Snapshot, error) {
	published, err := BuildSBTIPublishedSnapshot(model)
	if err != nil {
		return nil, err
	}
	return LegacySnapshotFromPublished(published), nil
}

func RefFromSnapshot(snapshot *domain.Snapshot) port.Ref {
	if snapshot == nil {
		return port.Ref{}
	}
	ref := port.Ref{
		Kind:    snapshot.Definition.Kind,
		Code:    snapshot.Definition.Code,
		Version: snapshot.Definition.Version,
		Title:   snapshot.Definition.Title,
	}
	if kind, subKind, algorithm, ok := domain.LegacyKindMapping(snapshot.Definition.Kind); ok {
		ref.Kind = kind
		ref.SubKind = subKind
		ref.Algorithm = algorithm
		return ref
	}
	if domain.IsPersonalityTypologyPayloadFormat(snapshot.PayloadFormat) {
		algorithm, err := domain.AlgorithmFromTypologyPayload(snapshot.Payload)
		if err == nil {
			ref.Kind = domain.KindPersonality
			ref.SubKind = domain.SubKindTypology
			ref.Algorithm = algorithm
		}
	}
	return ref
}

func canonicalRef(ref port.Ref) port.Ref {
	if kind, subKind, algorithm, ok := domain.LegacyKindMapping(ref.Kind); ok {
		ref.Kind = kind
		ref.SubKind = subKind
		ref.Algorithm = algorithm
	}
	return ref
}

// RefMatchesSnapshot reports whether ref points at the given legacy snapshot envelope.
func RefMatchesSnapshot(ref port.Ref, snapshot *domain.Snapshot) bool {
	if snapshot == nil || ref.Code == "" || ref.Version == "" {
		return false
	}
	if snapshot.Definition.Code != ref.Code || snapshot.Definition.Version != ref.Version {
		return false
	}
	want := canonicalRef(ref)
	got := RefFromSnapshot(snapshot)
	return want.Kind == got.Kind &&
		want.SubKind == got.SubKind &&
		want.Algorithm == got.Algorithm
}
