package modelcatalog

import "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/legacy"

// LegacyKindMapping resolves deprecated flat kinds to v2 identity triples.
func LegacyKindMapping(kind Kind) (Kind, SubKind, Algorithm, bool) {
	mappedKind, subKind, algorithm, ok := legacy.KindMapping(string(kind))
	if !ok {
		return "", "", "", false
	}
	return Kind(mappedKind), SubKind(subKind), Algorithm(algorithm), true
}

// ModelDefinitionFromLegacy builds a v2 definition from a v1 envelope definition.
func ModelDefinitionFromLegacy(def Definition, decision DecisionKind) ModelDefinition {
	if kind, subKind, algorithm, ok := LegacyKindMapping(def.Kind); ok {
		return ModelDefinition{
			Kind:      kind,
			SubKind:   subKind,
			Algorithm: algorithm,
			Code:      def.Code,
			Version:   def.Version,
			Title:     def.Title,
			Status:    def.Status,
		}
	}
	return ModelDefinition{
		Kind:    def.Kind,
		Code:    def.Code,
		Version: def.Version,
		Title:   def.Title,
		Status:  def.Status,
	}
}

// PublishedFromLegacy converts a v1 snapshot envelope to v2.
func PublishedFromLegacy(snapshot *Snapshot) *PublishedModelSnapshot {
	if snapshot == nil {
		return nil
	}
	source := SourceRef(nil)
	if snapshot.Source != nil {
		source = SourceRef(snapshot.Source)
	}
	return &PublishedModelSnapshot{
		SchemaVersion: SchemaVersionV2,
		Model:         ModelDefinitionFromLegacy(snapshot.Definition, snapshot.DecisionKind),
		Binding:       snapshot.Binding,
		Decision:      DecisionSpec{Kind: snapshot.DecisionKind},
		Source:        source,
		PayloadFormat: snapshot.PayloadFormat,
		Payload:       snapshot.Payload,
	}
}

// LegacyFromPublished converts a v2 snapshot to the v1 envelope for migration readers.
func LegacyFromPublished(snapshot *PublishedModelSnapshot) *Snapshot {
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
	case snapshot.Model.Kind == KindPersonality && snapshot.Model.Algorithm == AlgorithmMBTI:
		def.Kind = KindMBTIMigration
	case snapshot.Model.Kind == KindPersonality && snapshot.Model.Algorithm == AlgorithmSBTI:
		def.Kind = KindSBTIMigration
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
