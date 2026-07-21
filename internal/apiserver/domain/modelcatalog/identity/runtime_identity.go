package identity

// RuntimeIdentity is the publish-time frozen execution route for one AssessmentSnapshot.
type RuntimeIdentity struct {
	AlgorithmFamily AlgorithmFamily
	Algorithm       Algorithm
	DecisionKind    DecisionKind
}

// Complete reports whether all freeze-required fields are present.
func (r RuntimeIdentity) Complete() bool { return r.AlgorithmFamily != "" && r.DecisionKind != "" }
