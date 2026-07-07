package legacy

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/catalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/identity"
)

// LegacyKindMapping resolves deprecated flat kinds to v2 identity triples.
func LegacyKindMapping(kind identity.Kind) (identity.Kind, identity.SubKind, identity.Algorithm, bool) {
	mappedKind, subKind, algorithm, ok := KindMapping(string(kind))
	if !ok {
		return "", "", "", false
	}
	return identity.Kind(mappedKind), identity.SubKind(subKind), identity.Algorithm(algorithm), true
}

// ModelDefinitionFromLegacy builds a v2 definition from a v1 envelope definition.
func ModelDefinitionFromLegacy(def Definition, decision identity.DecisionKind) catalog.ModelDefinition {
	if kind, subKind, algorithm, ok := LegacyKindMapping(def.Kind); ok {
		return catalog.ModelDefinition{
			Kind:      kind,
			SubKind:   subKind,
			Algorithm: algorithm,
			Code:      def.Code,
			Version:   def.Version,
			Title:     def.Title,
			Status:    def.Status,
		}
	}
	return catalog.ModelDefinition{
		Kind:    def.Kind,
		Code:    def.Code,
		Version: def.Version,
		Title:   def.Title,
		Status:  def.Status,
	}
}

// PublishedFromLegacy converts a v1 snapshot envelope to v2.
func PublishedFromLegacy(snapshot *Snapshot) *catalog.PublishedModelSnapshot {
	if snapshot == nil {
		return nil
	}
	source := catalog.SourceRef(nil)
	if snapshot.Source != nil {
		source = catalog.SourceRef(snapshot.Source)
	}
	return &catalog.PublishedModelSnapshot{
		SchemaVersion: catalog.SchemaVersionV2,
		Model:         ModelDefinitionFromLegacy(snapshot.Definition, snapshot.DecisionKind),
		Binding:       snapshot.Binding,
		Decision:      catalog.DecisionSpec{Kind: snapshot.DecisionKind},
		Source:        source,
		PayloadFormat: snapshot.PayloadFormat,
		Payload:       snapshot.Payload,
	}
}

// LegacyFromPublished converts a v2 snapshot to the v1 envelope for migration readers.
func LegacyFromPublished(snapshot *catalog.PublishedModelSnapshot) *Snapshot {
	if snapshot == nil {
		return nil
	}
	def := Definition{
		Code:    snapshot.Model.Code,
		Version: snapshot.Model.Version,
		Title:   snapshot.Model.Title,
		Status:  snapshot.Model.Status,
	}
	switch {
	case snapshot.Model.Kind == identity.KindPersonality && snapshot.Model.Algorithm == identity.AlgorithmMBTI:
		def.Kind = identity.KindMBTIMigration
	case snapshot.Model.Kind == identity.KindPersonality && snapshot.Model.Algorithm == identity.AlgorithmSBTI:
		def.Kind = identity.KindSBTIMigration
	default:
		def.Kind = snapshot.Model.Kind
	}
	source := map[string]any(nil)
	if snapshot.Source != nil {
		source = map[string]any(snapshot.Source)
	}
	return &Snapshot{
		SchemaVersion: snapshot.SchemaVersion,
		PayloadFormat: snapshot.PayloadFormat,
		Definition:    def,
		Binding:       snapshot.Binding,
		DecisionKind:  snapshot.Decision.Kind,
		Source:        source,
		Payload:       snapshot.Payload,
	}
}
