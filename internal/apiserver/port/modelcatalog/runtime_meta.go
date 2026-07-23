package modelcatalog

import domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"

// PublishedRuntimeMeta carries AssessmentSnapshot RuntimeIdentity for evaluation input
// materialization. It is not part of the published payload JSON envelope.
type PublishedRuntimeMeta struct {
	Kind         domain.Kind
	Algorithm    domain.Algorithm
	DecisionKind domain.DecisionKind
}

// RuntimeMetaFromPublished builds evaluation-only runtime metadata from a published snapshot.
// Published snapshots are DefinitionV2-only and must already carry a complete route.
func RuntimeMetaFromPublished(model *PublishedModel) *PublishedRuntimeMeta {
	if model == nil {
		return nil
	}
	return &PublishedRuntimeMeta{
		Kind:         model.Kind,
		Algorithm:    model.Algorithm,
		DecisionKind: model.DecisionKind,
	}
}
