package identity

// AlgorithmWritePolicy classifies whether an algorithm may be used on new writes (MC-R018).
type AlgorithmWritePolicy string

const (
	// AlgorithmWriteCanonical is allowed for new draft create defaults and publish.
	AlgorithmWriteCanonical AlgorithmWritePolicy = "canonical"
	// AlgorithmWriteDraftOK allows empty Algorithm during draft authoring only.
	AlgorithmWriteDraftOK AlgorithmWritePolicy = "draft_ok"
	// AlgorithmWriteUnknown is neither a known draft, publish, nor retained-read value.
	AlgorithmWriteUnknown AlgorithmWritePolicy = "unknown"
)

// ClassifyAlgorithmWritePolicy reports the write/read lifecycle for Kind+Algorithm.
// CompatibleAlgorithmBinding remains the draft/read matrix; this policy separates
// new-write (canonical/draft_ok) from retired aliases (unknown).
func ClassifyAlgorithmWritePolicy(kind Kind, algorithm Algorithm) AlgorithmWritePolicy {
	switch kind {
	case KindScale:
		switch algorithm {
		case AlgorithmScaleDefault:
			return AlgorithmWriteCanonical
		case "":
			return AlgorithmWriteDraftOK
		default:
			return AlgorithmWriteUnknown
		}
	case KindTypology:
		switch algorithm {
		case AlgorithmPersonalityTypology:
			return AlgorithmWriteCanonical
		case "":
			return AlgorithmWriteDraftOK
		default:
			return AlgorithmWriteUnknown
		}
	case KindBehavioralRating:
		switch algorithm {
		case AlgorithmBrief2, AlgorithmSPMSensory:
			return AlgorithmWriteCanonical
		case "":
			return AlgorithmWriteDraftOK
		default:
			return AlgorithmWriteUnknown
		}
	case KindCognitive:
		switch algorithm {
		case AlgorithmSPM:
			return AlgorithmWriteCanonical
		case "":
			return AlgorithmWriteDraftOK
		default:
			return AlgorithmWriteUnknown
		}
	default:
		return AlgorithmWriteUnknown
	}
}

// IsCanonicalPublishAlgorithm reports whether a new publish may persist this algorithm.
func IsCanonicalPublishAlgorithm(kind Kind, algorithm Algorithm) bool {
	return ClassifyAlgorithmWritePolicy(kind, algorithm) == AlgorithmWriteCanonical
}

// IdentityAuditIssue records a published or runtime identity that is not canonical.
type IdentityAuditIssue struct {
	Code      string
	Message   string
	Kind      Kind
	Algorithm Algorithm
	Policy    AlgorithmWritePolicy
}

// AuditIdentityWritePolicy classifies Kind/Algorithm for inventory and retirement gates.
func AuditIdentityWritePolicy(kind Kind, algorithm Algorithm) []IdentityAuditIssue {
	policy := ClassifyAlgorithmWritePolicy(kind, algorithm)
	switch policy {
	case AlgorithmWriteCanonical:
		return nil
	case AlgorithmWriteDraftOK:
		return []IdentityAuditIssue{{
			Code: "identity.algorithm.empty", Message: "algorithm is empty; draft-only, not publishable",
			Kind: kind, Algorithm: algorithm, Policy: policy,
		}}
	default:
		return []IdentityAuditIssue{{
			Code: "identity.algorithm.unknown", Message: "algorithm is not a known identity binding",
			Kind: kind, Algorithm: algorithm, Policy: policy,
		}}
	}
}
