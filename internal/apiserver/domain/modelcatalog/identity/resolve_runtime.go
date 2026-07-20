package identity

import "fmt"

// ResolveRuntimeIdentity validates and freezes the runtime route at publish time.
// Identity-derived family and decision-derived family must agree when both resolve.
// Legacy decode-only payload formats are rejected here without importing payloadformat
// (that package depends on binding which depends on identity).
func ResolveRuntimeIdentity(kind Kind, subKind SubKind, algorithm Algorithm, decision DecisionKind, format string) (RuntimeIdentity, error) {
	if decision == "" {
		return RuntimeIdentity{}, fmt.Errorf("decision_kind is required to freeze runtime identity")
	}
	if format == "" {
		return RuntimeIdentity{}, fmt.Errorf("payload_format is required to freeze runtime identity")
	}
	if isLegacyDecodeOnlyPayloadFormat(format) {
		return RuntimeIdentity{}, fmt.Errorf("payload_format %q is legacy decode-only and cannot be published", format)
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
		PayloadFormat:   format,
	}, nil
}

// Keep in sync with payloadformat.LegacyDecodeOnlyPayloadFormats.
func isLegacyDecodeOnlyPayloadFormat(format string) bool {
	switch format {
	case "ruleset.scale.v1",
		"ruleset.mbti.v1",
		"ruleset.sbti.v1",
		"evaluationinput.scale.v1",
		"evaluationinput.mbti.v1",
		"evaluationinput.sbti.v1",
		"assessmentmodel.behavioral_rating.brief2.v1",
		"assessmentmodel.cognitive.spm.v1":
		return true
	default:
		return false
	}
}
