package identity

import "fmt"

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
			return RuntimeIdentity{}, fmt.Errorf("legacy runtime identity cannot derive decision_kind for %s/%s", kind, algorithm)
		}
	}
	family, ok := AlgorithmFamilyFromDecisionKind(decision)
	if !ok {
		return RuntimeIdentity{}, fmt.Errorf("decision_kind %q does not map to an algorithm family", decision)
	}
	if expected, known := AlgorithmFamilyFromIdentity(kind, subKind, algorithm); known && expected != family {
		return RuntimeIdentity{}, fmt.Errorf("legacy runtime identity conflict: %s/%s => %s, decision_kind %s => %s", kind, algorithm, expected, decision, family)
	}
	return RuntimeIdentity{Algorithm: algorithm, DecisionKind: decision, AlgorithmFamily: family}, nil
}
