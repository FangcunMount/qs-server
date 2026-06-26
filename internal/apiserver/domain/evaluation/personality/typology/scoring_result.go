package typology

// ScoringResult is the domain-local output of a personality model adapter.
// Detail holds algorithm-specific payload such as MBTIResultDetail or SBTIResultDetail.
type ScoringResult struct {
	Detail any
}
