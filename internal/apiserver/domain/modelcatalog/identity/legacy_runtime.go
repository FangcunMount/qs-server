package identity

import (
	"errors"
	"fmt"
)

// ErrLegacyRuntimeIdentity is returned whenever a historical record cannot be
// admitted to an executable route. Callers may still render its static body,
// but must not rebuild, retry, or dispatch it through a descriptor.
var ErrLegacyRuntimeIdentity = errors.New("legacy-runtime-identity")

// ResolveLegacyRuntime normalizes a persisted runtime identity without ever
// trusting a historical algorithm_family value. DecisionKind is canonical; a
// missing value may be restored only when Kind+Algorithm maps uniquely.
func ResolveLegacyRuntime(kind Kind, algorithm Algorithm, decision DecisionKind) (RuntimeIdentity, error) {
	subKind := SubKindEmpty
	if kind == KindTypology {
		subKind = SubKindTypology
	}
	if decision == "" {
		var ok bool
		decision, ok = DecisionKindForIdentity(kind, subKind, algorithm)
		if !ok || decision == "" {
			return RuntimeIdentity{}, fmt.Errorf("%w: cannot derive decision_kind for %s/%s", ErrLegacyRuntimeIdentity, kind, algorithm)
		}
	}
	family, ok := AlgorithmFamilyFromDecisionKind(decision)
	if !ok {
		return RuntimeIdentity{}, fmt.Errorf("%w: decision_kind %q does not map to an algorithm family", ErrLegacyRuntimeIdentity, decision)
	}
	if expected, known := AlgorithmFamilyFromIdentity(kind, subKind, algorithm); known && expected != family {
		return RuntimeIdentity{}, fmt.Errorf("%w: conflict %s/%s => %s, decision_kind %s => %s", ErrLegacyRuntimeIdentity, kind, algorithm, expected, decision, family)
	}
	return RuntimeIdentity{Algorithm: algorithm, DecisionKind: decision, AlgorithmFamily: family}, nil
}
