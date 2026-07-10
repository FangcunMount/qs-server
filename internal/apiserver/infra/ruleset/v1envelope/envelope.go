package v1envelope

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/payloadformat"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
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
	Binding       binding.QuestionnaireBinding
	DecisionKind  binding.DecisionKind
	Source        map[string]any
	Payload       []byte
}

// RuleSetSnapshot is a compatibility alias for V1Snapshot.
type RuleSetSnapshot = V1Snapshot

// RuleSetDefinition is a compatibility alias for V1Definition.
type RuleSetDefinition = V1Definition

const RuleSetSchemaVersionV1 = payloadformat.SchemaVersionV1

// PublishedFromV1 converts a v1 snapshot envelope to a v2 published model record.
func PublishedFromV1(snapshot *V1Snapshot) *port.PublishedModel {
	if snapshot == nil {
		return nil
	}
	source := map[string]any(nil)
	if snapshot.Source != nil {
		source = map[string]any(snapshot.Source)
	}
	kind, subKind, algorithm := identityFromV1(snapshot.Definition)
	return &port.PublishedModel{
		SchemaVersion:        payloadformat.SchemaVersionV2,
		ProductChannel:       binding.DefaultProductChannelFor(kind),
		Kind:                 kind,
		SubKind:              subKind,
		Algorithm:            algorithm,
		Code:                 snapshot.Definition.Code,
		Version:              snapshot.Definition.Version,
		Title:                snapshot.Definition.Title,
		Status:               snapshot.Definition.Status,
		QuestionnaireCode:    snapshot.Binding.QuestionnaireCode,
		QuestionnaireVersion: snapshot.Binding.QuestionnaireVersion,
		DecisionKind:         snapshot.DecisionKind,
		Source:               source,
		PayloadFormat:        snapshot.PayloadFormat,
		Payload:              snapshot.Payload,
	}
}

// V1FromPublished converts v2 published record back to v1 envelope for codec/oneoff tests.
func V1FromPublished(model *port.PublishedModel) *V1Snapshot {
	if model == nil {
		return nil
	}
	source := map[string]any(nil)
	if model.Source != nil {
		source = map[string]any(model.Source)
	}
	return &V1Snapshot{
		SchemaVersion: model.SchemaVersion,
		PayloadFormat: model.PayloadFormat,
		Definition: V1Definition{
			Kind:    model.Kind,
			Code:    model.Code,
			Version: model.Version,
			Title:   model.Title,
			Status:  model.Status,
		},
		Binding: binding.QuestionnaireBinding{
			QuestionnaireCode:    model.QuestionnaireCode,
			QuestionnaireVersion: model.QuestionnaireVersion,
		},
		DecisionKind: model.DecisionKind,
		Source:       source,
		Payload:      model.Payload,
	}
}

func identityFromV1(def V1Definition) (binding.Kind, binding.SubKind, binding.Algorithm) {
	if kind, subKind, algorithm, ok := binding.LegacyKindMapping(def.Kind); ok {
		return kind, subKind, algorithm
	}
	return def.Kind, "", ""
}
