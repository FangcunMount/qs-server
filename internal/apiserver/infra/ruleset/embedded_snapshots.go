package ruleset

import (
	"context"
	"encoding/json"
	"fmt"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/typology"
)

// DefaultEmbeddedRuleSets builds v2 published snapshots from embedded SBTI/MBTI seed.
func DefaultEmbeddedRuleSets(ctx context.Context) ([]*port.PublishedModel, error) {
	return defaultEmbeddedSnapshots(ctx)
}

// DefaultEmbeddedSnapshots builds v2 published snapshots from embedded SBTI/MBTI seed.
func DefaultEmbeddedSnapshots(ctx context.Context) ([]*port.PublishedModel, error) {
	return defaultEmbeddedSnapshots(ctx)
}

func defaultEmbeddedSnapshots(_ context.Context) ([]*port.PublishedModel, error) {
	sbtiModel, err := LoadDefaultSBTILegacyModel()
	if err != nil {
		return nil, err
	}
	mbtiModel, err := LoadDefaultMBTILegacyModel()
	if err != nil {
		return nil, err
	}
	sbtiSnapshot, err := publishedSnapshotFromEmbeddedTypology(modeltypology.FromSBTI(sbtiModel))
	if err != nil {
		return nil, err
	}
	mbtiSnapshot, err := publishedSnapshotFromEmbeddedTypology(modeltypology.FromMBTI(mbtiModel))
	if err != nil {
		return nil, err
	}
	return []*port.PublishedModel{sbtiSnapshot, mbtiSnapshot}, nil
}

// publishedSnapshotFromEmbeddedTypology is an infrastructure seed adapter.
// It converts embedded legacy fixtures once into the canonical DefinitionV2
// runtime record; normal publishing always goes through definition.Registry.
func publishedSnapshotFromEmbeddedTypology(payload *modeltypology.Payload) (*port.PublishedModel, error) {
	if payload == nil {
		return nil, fmt.Errorf("embedded typology payload is nil")
	}
	runtime, err := payload.ToRuntimeSpec()
	if err != nil {
		return nil, fmt.Errorf("build embedded typology runtime: %w", err)
	}
	definition := modeltypology.DefinitionFromRuntime(payload, runtime)
	decisionKind, err := (&domain.AssessmentModel{Kind: domain.KindTypology, DefinitionV2: definition}).DecisionKindForDefinition()
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal embedded typology payload: %w", err)
	}
	status := payload.Status
	if status == "" {
		status = string(domain.ModelStatusPublished)
	}
	return &port.PublishedModel{
		SchemaVersion:        domain.SchemaVersionV2,
		PayloadFormat:        domain.PayloadFormatPersonalityTypologyV1,
		ProductChannel:       domain.ProductChannelTypology,
		Kind:                 domain.KindTypology,
		SubKind:              domain.SubKindTypology,
		Algorithm:            payload.Algorithm,
		Code:                 payload.Code,
		Version:              payload.Version,
		Title:                payload.Title,
		Status:               status,
		DecisionKind:         decisionKind,
		QuestionnaireCode:    payload.QuestionnaireCode,
		QuestionnaireVersion: payload.QuestionnaireVersion,
		Source: map[string]any{
			"questions_repo": payload.Source.QuestionsRepo,
			"wiki_repo":      payload.Source.WikiRepo,
			"source_site":    payload.Source.SourceSite,
			"license":        payload.Source.License,
			"attribution":    payload.Source.Attribution,
			"image_base_url": payload.Source.ImageBaseURL,
			"non_commercial": payload.Source.NonCommercial,
		},
		Payload:      encoded,
		DefinitionV2: definition,
	}, nil
}
