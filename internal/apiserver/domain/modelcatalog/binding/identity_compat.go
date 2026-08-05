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
