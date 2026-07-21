package identity

import "fmt"

// ResolveRuntimeIdentity validates and freezes the runtime route at publish time.
// Identity-derived family and decision-derived family must agree when both resolve.
func ResolveRuntimeIdentity(kind Kind, subKind SubKind, algorithm Algorithm, decision DecisionKind) (RuntimeIdentity, error) {
	if decision == "" {
		return RuntimeIdentity{}, fmt.Errorf("decision_kind is required to freeze runtime identity")
	}
	familyFromDecision, ok := AlgorithmFamilyFromDecisionKind(decision)
	if !ok {
		return RuntimeIdentity{}, fmt.Errorf("decision_kind %q does not map to an algorithm family", decision)
	}
	if familyFromIdentity, ok := AlgorithmFamilyFromIdentity(kind, subKind, algorithm); ok && familyFromIdentity != familyFromDecision {
		return RuntimeIdentity{}, fmt.Errorf(
			"runtime identity conflict: identity %s/%s/%s => family %s, decision_kind %s => family %s",
			kind, subKind, algorithm, familyFromIdentity, decision, familyFromDecision,
		)
	}
	return RuntimeIdentity{
		AlgorithmFamily: familyFromDecision,
		Algorithm:       algorithm,
		DecisionKind:    decision,
	}, nil
}
