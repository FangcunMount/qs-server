package definition

import (
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// AlgorithmBinding is the registry resolution key: ModelIdentity fields plus
// the derived AlgorithmFamily from the compatibility matrix.
type AlgorithmBinding struct {
	Kind      domain.Kind
	SubKind   domain.SubKind
	Algorithm domain.Algorithm
	Family    domain.AlgorithmFamily // optional; derived when empty and matrix matches
}

// AlgorithmBindingFromIdentity builds a binding key from draft Identity fields.
func AlgorithmBindingFromIdentity(identity domain.Identity) AlgorithmBinding {
	return AlgorithmBinding{
		Kind:      identity.Kind,
		SubKind:   identity.SubKind,
		Algorithm: identity.Algorithm,
	}.WithDerivedFamily()
}

// AlgorithmBindingFromModel builds a binding key from an AssessmentModel head.
func AlgorithmBindingFromModel(model *domain.AssessmentModel) AlgorithmBinding {
	if model == nil {
		return AlgorithmBinding{}
	}
	return AlgorithmBindingFromIdentity(domain.Identity{
		Kind: model.Kind, SubKind: domain.CanonicalSubKindFor(model.Kind), Algorithm: model.Algorithm,
	})
}

// Identity returns the Kind/SubKind/Algorithm triple used by Handler.Supports.
func (b AlgorithmBinding) Identity() domain.Identity {
	return domain.Identity{Kind: b.Kind, SubKind: b.SubKind, Algorithm: b.Algorithm}
}

// WithDerivedFamily fills Family from the compatibility matrix when empty.
func (b AlgorithmBinding) WithDerivedFamily() AlgorithmBinding {
	if b.Family != "" {
		return b
	}
	if family, ok := domain.AlgorithmFamilyFromIdentity(b.Kind, b.SubKind, b.Algorithm); ok {
		b.Family = family
	}
	return b
}

// Compatible reports whether this binding is allowed by the Kind↔Algorithm matrix.
func (b AlgorithmBinding) Compatible() bool {
	return domain.CompatibleAlgorithmBinding(b.Kind, b.SubKind, b.Algorithm)
}

// supportsBinding is the shared Supports predicate: Kind match + matrix entry.
func supportsBinding(kind domain.Kind, identity domain.Identity) bool {
	if identity.Kind != kind {
		return false
	}
	return domain.CompatibleAlgorithmBinding(identity.Kind, identity.SubKind, identity.Algorithm)
}
