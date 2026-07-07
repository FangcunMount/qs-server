package factor

// FactorSnapshot is the canonical published-model dimension definition.
type FactorSnapshot struct {
	Code            string
	Title           string
	Role            FactorRole
	IsTotalScore    bool
	QuestionCodes   []string
	ScoringStrategy string
	ScoringParams   *ScoringParams
	MaxScore        *float64
	InterpretRules  []ScoreRangeRule
	Classification  *ClassificationSpec
	Norm            *NormRef
}

// ResolvedRole returns the explicit role or derives one from legacy flags.
func (f FactorSnapshot) ResolvedRole() FactorRole {
	if f.Role != "" {
		return f.Role
	}
	if f.IsTotalScore {
		return FactorRoleTotal
	}
	return FactorRoleDimension
}
