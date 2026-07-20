package binding

import identitypkg "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/identity"

// DecisionKindForIdentity derives a decision from canonical model identity.
func DecisionKindForIdentity(kind Kind, subKind SubKind, algorithm Algorithm) (DecisionKind, bool) {
	return identitypkg.DecisionKindForIdentity(kind, subKind, algorithm)
}

// AlgorithmFamilyFromIdentity derives a runtime family from canonical identity.
func AlgorithmFamilyFromIdentity(kind Kind, subKind SubKind, algorithm Algorithm) (identitypkg.AlgorithmFamily, bool) {
	return identitypkg.AlgorithmFamilyFromIdentity(kind, subKind, algorithm)
}

// AlgorithmFamilyStringFromIdentity normalizes persisted kinds before deriving a runtime family string.
func AlgorithmFamilyStringFromIdentity(kind Kind, subKind SubKind, algorithm Algorithm) string {
	family, ok := AlgorithmFamilyFromIdentity(kind, subKind, algorithm)
	if !ok {
		return ""
	}
	return string(family)
}

// CompatibleAlgorithmBinding reports whether Kind/SubKind/Algorithm form a
// known ModelIdentity ↔ AlgorithmBinding matrix entry.
func CompatibleAlgorithmBinding(kind Kind, subKind SubKind, algorithm Algorithm) bool {
	return identitypkg.CompatibleAlgorithmBinding(kind, subKind, algorithm)
}
