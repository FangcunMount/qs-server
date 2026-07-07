package legacy

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/catalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/identity"
)

// LegacyKindMapping 解析deprecated flat 类型 到 v2 身份 triples。
func LegacyKindMapping(kind identity.Kind) (identity.Kind, identity.SubKind, identity.Algorithm, bool) {
	mappedKind, subKind, algorithm, ok := KindMapping(string(kind))
	if !ok {
		return "", "", "", false
	}
	return identity.Kind(mappedKind), identity.SubKind(subKind), identity.Algorithm(algorithm), true
}

// ModelDefinitionFromLegacy 构建v2 definition 从 v1 envelope definition。
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

// PublishedFromLegacy 转换v1 快照 envelope 到 v2。
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

// LegacyFromPublished 转换v2 快照 到 v1 envelope 用于 迁移 读取器。
func LegacyFromPublished(snapshot *catalog.PublishedModelSnapshot) *Snapshot {
	if snapshot == nil {
		return nil
	}
	def := Definition{
		Kind:    snapshot.Model.Kind,
		Code:    snapshot.Model.Code,
		Version: snapshot.Model.Version,
		Title:   snapshot.Model.Title,
		Status:  snapshot.Model.Status,
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
