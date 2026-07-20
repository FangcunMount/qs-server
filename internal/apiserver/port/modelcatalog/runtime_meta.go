package modelcatalog

import domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"

// PublishedRuntimeMeta carries AssessmentSnapshot RuntimeIdentity for evaluation input
// materialization. It is not part of the published payload JSON envelope.
type PublishedRuntimeMeta struct {
	AlgorithmFamily domain.AlgorithmFamily
	DecisionKind    domain.DecisionKind
	PayloadFormat   string
	ProductChannel  domain.ProductChannel
	Kind            domain.Kind
	SubKind         domain.SubKind
	Algorithm       domain.Algorithm
}

// RuntimeMetaFromPublished builds evaluation-only runtime metadata from a published snapshot.
// Old snapshots missing AlgorithmFamily are filled via explicit compatibility derivation.
func RuntimeMetaFromPublished(model *PublishedModel) *PublishedRuntimeMeta {
	if model == nil {
		return nil
	}
	family := model.AlgorithmFamily
	compat := false
	if family == "" {
		compat = true
		if f, ok := domain.AlgorithmFamilyFromDecisionKind(model.DecisionKind); ok {
			family = f
		} else if f, ok := domain.AlgorithmFamilyFromIdentity(model.Kind, model.SubKind, model.Algorithm); ok {
			family = f
		}
	}
	decision := model.DecisionKind
	format := model.PayloadFormat
	if format == "" {
		compat = true
		format = domain.DraftPayloadFormatForModel(model.Kind, model.Algorithm)
	}
	_ = compat
	return &PublishedRuntimeMeta{
		AlgorithmFamily: family,
		DecisionKind:    decision,
		PayloadFormat:   format,
		ProductChannel:  model.ProductChannel,
		Kind:            model.Kind,
		SubKind:         model.SubKind,
		Algorithm:       model.Algorithm,
	}
}
