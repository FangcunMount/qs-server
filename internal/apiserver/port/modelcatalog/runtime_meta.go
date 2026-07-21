package modelcatalog

import domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"

// PublishedRuntimeMeta carries AssessmentSnapshot RuntimeIdentity for evaluation input
// materialization. It is not part of the published payload JSON envelope.
type PublishedRuntimeMeta struct {
	AlgorithmFamily domain.AlgorithmFamily
	DecisionKind    domain.DecisionKind
	ProductChannel  domain.ProductChannel
	Kind            domain.Kind
	SubKind         domain.SubKind
	Algorithm       domain.Algorithm
}

// RuntimeMetaFromPublished builds evaluation-only runtime metadata from a published snapshot.
// Published snapshots are DefinitionV2-only and must already carry a complete route.
func RuntimeMetaFromPublished(model *PublishedModel) *PublishedRuntimeMeta {
	if model == nil {
		return nil
	}
	return &PublishedRuntimeMeta{
		AlgorithmFamily: model.AlgorithmFamily,
		DecisionKind:    model.DecisionKind,
		ProductChannel:  model.ProductChannel,
		Kind:            model.Kind,
		SubKind:         model.SubKind,
		Algorithm:       model.Algorithm,
	}
}
