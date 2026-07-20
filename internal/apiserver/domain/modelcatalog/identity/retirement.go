package identity

// RetirementGateInputs are inventory / attestation inputs for MC-R018 batch 5.
// Metrics attestation is ops-owned (Prometheus); the gate does not scrape metrics.
type RetirementGateInputs struct {
	PublishedRetainedRead    int
	AssessmentRetainedAlias  int  // mbti|sbti|bigfive|behavioral_rating_default
	AssessmentEmptyAlgorithm int  // empty/NULL algorithm rows (separate from alias)
	MetricsRetainedReadOK    bool // rate(policy="retained_read"[14d]) ≈ 0
	MetricsFallbackOK        bool // rate(algorithm_fallback_total[14d]) ≈ 0
}

// RetirementGate reports whether compatibility branches may be deleted.
type RetirementGate struct {
	Status  string   // PASS | FAIL | WARN
	Reasons []string `json:",omitempty"`
}

// EvaluateRetirementGate encodes the full MC-R018 delete precondition
// (dual-identity + empty-algorithm fallback + metrics).
func EvaluateRetirementGate(in RetirementGateInputs) RetirementGate {
	var reasons []string
	if in.PublishedRetainedRead > 0 {
		reasons = append(reasons, "published_retained_read>0")
	}
	if in.AssessmentRetainedAlias > 0 {
		reasons = append(reasons, "assessment_retained_alias>0")
	}
	if in.AssessmentEmptyAlgorithm > 0 {
		reasons = append(reasons, "assessment_empty_algorithm>0")
	}
	if len(reasons) > 0 {
		return RetirementGate{Status: "FAIL", Reasons: reasons}
	}
	return attestMetrics(in)
}

// EvaluateDualIdentityRetirementGate is the narrower gate for deleting
// dual-identity / retained-alias lookup only. Empty-algorithm inventory is ignored.
func EvaluateDualIdentityRetirementGate(in RetirementGateInputs) RetirementGate {
	var reasons []string
	if in.PublishedRetainedRead > 0 {
		reasons = append(reasons, "published_retained_read>0")
	}
	if in.AssessmentRetainedAlias > 0 {
		reasons = append(reasons, "assessment_retained_alias>0")
	}
	if len(reasons) > 0 {
		return RetirementGate{Status: "FAIL", Reasons: reasons}
	}
	return attestMetrics(in)
}

func attestMetrics(in RetirementGateInputs) RetirementGate {
	var reasons []string
	if !in.MetricsRetainedReadOK {
		reasons = append(reasons, "metrics_retained_read_not_attested")
	}
	if !in.MetricsFallbackOK {
		reasons = append(reasons, "metrics_algorithm_fallback_not_attested")
	}
	if len(reasons) > 0 {
		return RetirementGate{Status: "WARN", Reasons: reasons}
	}
	return RetirementGate{Status: "PASS"}
}

// IsRetainedReadAliasAlgorithm reports Assessment/Outcome values that require
// dual-identity lookup (independent of Kind, including legacy personality).
func IsRetainedReadAliasAlgorithm(algorithm Algorithm) bool {
	switch algorithm {
	case AlgorithmMBTI, AlgorithmSBTI, AlgorithmBigFive, AlgorithmBehavioralRatingDefault:
		return true
	default:
		return false
	}
}

// DualIdentityDeleteChecklist is empty after dual-identity retirement (MC-R018).
// Prefer FullDeleteChecklist / EvaluateRetirementGate for remaining empty-algorithm work.
func DualIdentityDeleteChecklist() []string {
	return nil
}

// RetirementDeleteChecklist lists remaining compatibility surfaces after dual-identity
// retirement. Remove only after EvaluateRetirementGate returns PASS.
func RetirementDeleteChecklist() []string {
	return []string{
		"compat ModelPayload decoder retained-read paths (MC-R017 overlap)",
		"ExecutionIdentityBehavioralRatingDefault / CognitiveDefault family route key remap",
		"oneoff audit_assessment_retained_algorithms.sql",
		"oneoff soft_delete_assessment_empty_algorithms (full_gate inventory)",
	}
}
