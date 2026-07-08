package v1envelope

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/publishing"
)

// V1 migration-only flat kinds.
const (
	KindMBTIMigration = "mbti"
	KindSBTIMigration = "sbti"
)

// RuleSetKind is kept for v1 codec/oneoff compatibility.
type RuleSetKind = binding.Kind

const (
	RuleSetKindScale = binding.KindScale
	RuleSetKindMBTI  = RuleSetKind(KindMBTIMigration)
	RuleSetKindSBTI  = RuleSetKind(KindSBTIMigration)
)

// V1Definition is the v1 ruleset envelope definition block.
type V1Definition struct {
	Kind    binding.Kind
	Code    string
	Version string
	Title   string
	Status  string
}

// V1Snapshot is the legacy evaluation_rulesets envelope. Codec/oneoff only.
type V1Snapshot struct {
	SchemaVersion string
	PayloadFormat string
	Definition    V1Definition
	Binding       publishing.QuestionnaireBinding
	DecisionKind  binding.DecisionKind
	Source        map[string]any
	Payload       []byte
}

// RuleSetSnapshot is a compatibility alias for V1Snapshot.
type RuleSetSnapshot = V1Snapshot

// RuleSetDefinition is a compatibility alias for V1Definition.
type RuleSetDefinition = V1Definition

const RuleSetSchemaVersionV1 = publishing.SchemaVersionV1

// PublishedFromV1 converts a v1 snapshot envelope to v2 published model snapshot.
func PublishedFromV1(snapshot *V1Snapshot) *publishing.PublishedModelSnapshot {
	if snapshot == nil {
		return nil
	}
	source := publishing.SourceRef(nil)
	if snapshot.Source != nil {
		source = publishing.SourceRef(snapshot.Source)
	}
	return &publishing.PublishedModelSnapshot{
		SchemaVersion: publishing.SchemaVersionV2,
		Model:         modelDefinitionFromV1(snapshot.Definition, snapshot.DecisionKind),
		Binding:       snapshot.Binding,
		Decision:      publishing.DecisionSpec{Kind: snapshot.DecisionKind},
		Source:        source,
		PayloadFormat: snapshot.PayloadFormat,
		Payload:       snapshot.Payload,
	}
}

// V1FromPublished converts v2 snapshot back to v1 envelope for codec/oneoff tests.
func V1FromPublished(snapshot *publishing.PublishedModelSnapshot) *V1Snapshot {
	if snapshot == nil {
		return nil
	}
	source := map[string]any(nil)
	if snapshot.Source != nil {
		source = map[string]any(snapshot.Source)
	}
	return &V1Snapshot{
		SchemaVersion: snapshot.SchemaVersion,
		PayloadFormat: snapshot.PayloadFormat,
		Definition: V1Definition{
			Kind:    snapshot.Model.Kind,
			Code:    snapshot.Model.Code,
			Version: snapshot.Model.Version,
			Title:   snapshot.Model.Title,
			Status:  snapshot.Model.Status,
		},
		Binding:      snapshot.Binding,
		DecisionKind: snapshot.Decision.Kind,
		Source:       source,
		Payload:      snapshot.Payload,
	}
}

func modelDefinitionFromV1(def V1Definition, decision binding.DecisionKind) publishing.ModelDefinition {
	if kind, subKind, algorithm, ok := binding.LegacyKindMapping(def.Kind); ok {
		return publishing.ModelDefinition{
			Kind:      kind,
			SubKind:   subKind,
			Algorithm: algorithm,
			Code:      def.Code,
			Version:   def.Version,
			Title:     def.Title,
			Status:    def.Status,
		}
	}
	kind := binding.NormalizeKind(def.Kind)
	return publishing.ModelDefinition{
		Kind:    kind,
		Code:    def.Code,
		Version: def.Version,
		Title:   def.Title,
		Status:  def.Status,
	}
}
