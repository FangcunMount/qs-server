package snapshot

import "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"

// ExecutionEnvelope carries non-factor metadata for scale-like execution.
type ExecutionEnvelope struct {
	ID                   uint64
	Code                 string
	ScaleVersion         string
	Title                string
	QuestionnaireCode    string
	QuestionnaireVersion string
	Status               string
}

// InterpretRuleFromScoreRange projects a canonical rule into scale execution shape.
func InterpretRuleFromScoreRange(r factor.ScoreRangeRule) InterpretRuleSnapshot {
	return InterpretRuleSnapshot{
		Min:        r.MinScore,
		Max:        r.MaxScore,
		RiskLevel:  r.Level,
		Conclusion: r.Conclusion,
		Suggestion: r.Suggestion,
	}
}

// ScoreRangeFromInterpretRule adapts a scale execution rule into canonical form.
func ScoreRangeFromInterpretRule(rule InterpretRuleSnapshot) factor.ScoreRangeRule {
	return factor.ScoreRangeRule{
		MinScore:   rule.Min,
		MaxScore:   rule.Max,
		Level:      rule.RiskLevel,
		Conclusion: rule.Conclusion,
		Suggestion: rule.Suggestion,
	}
}

// FactorFromCanonical projects one canonical factor into scale execution shape.
func FactorFromCanonical(f factor.FactorSnapshot) FactorSnapshot {
	rules := make([]InterpretRuleSnapshot, 0, len(f.InterpretRules))
	for _, rule := range f.InterpretRules {
		rules = append(rules, InterpretRuleFromScoreRange(rule))
	}
	var params ScoringParamsSnapshot
	if f.ScoringParams != nil {
		params.CntOptionContents = append([]string(nil), f.ScoringParams.CntOptionContents...)
	}
	return FactorSnapshot{
		Code:            f.Code,
		Title:           f.Title,
		IsTotalScore:    f.IsTotalScore,
		QuestionCodes:   append([]string(nil), f.QuestionCodes...),
		ScoringStrategy: f.ScoringStrategy,
		ScoringParams:   params,
		MaxScore:        f.MaxScore,
		InterpretRules:  rules,
	}
}

// Canonical adapts a scale execution factor into canonical catalog form.
func (f FactorSnapshot) Canonical() factor.FactorSnapshot {
	rules := make([]factor.ScoreRangeRule, 0, len(f.InterpretRules))
	for _, rule := range f.InterpretRules {
		rules = append(rules, ScoreRangeFromInterpretRule(rule))
	}
	var params *factor.ScoringParams
	if len(f.ScoringParams.CntOptionContents) > 0 {
		params = &factor.ScoringParams{
			CntOptionContents: append([]string(nil), f.ScoringParams.CntOptionContents...),
		}
	}
	role := factor.FactorRoleDimension
	if f.IsTotalScore {
		role = factor.FactorRoleTotal
	}
	return factor.FactorSnapshot{
		Code:            f.Code,
		Title:           f.Title,
		Role:            role,
		IsTotalScore:    f.IsTotalScore,
		QuestionCodes:   append([]string(nil), f.QuestionCodes...),
		ScoringStrategy: f.ScoringStrategy,
		ScoringParams:   params,
		MaxScore:        f.MaxScore,
		InterpretRules:  rules,
	}
}

// FactorSnapshotFromCanonical materializes a scale execution factor from canonical form.
func FactorSnapshotFromCanonical(f factor.FactorSnapshot) FactorSnapshot {
	return FactorFromCanonical(f)
}

// FactorsFromCanonical projects canonical factors into scale execution factors.
func FactorsFromCanonical(factors []factor.FactorSnapshot) []FactorSnapshot {
	out := make([]FactorSnapshot, 0, len(factors))
	for _, item := range factors {
		out = append(out, FactorFromCanonical(item))
	}
	return out
}

// BuildFromModelFactors materializes a scale snapshot from common family snapshot metadata.
func BuildFromModelFactors(code, version, title, questionnaireCode, questionnaireVersion, status string, factors []factor.FactorSnapshot) *ScaleSnapshot {
	return BuildFromCanonicalFactors(ExecutionEnvelope{
		Code:                 code,
		ScaleVersion:         version,
		Title:                title,
		QuestionnaireCode:    questionnaireCode,
		QuestionnaireVersion: questionnaireVersion,
		Status:               status,
	}, factors)
}

// BuildFromCanonicalFactors materializes a scale execution snapshot from canonical factors.
func BuildFromCanonicalFactors(env ExecutionEnvelope, factors []factor.FactorSnapshot) *ScaleSnapshot {
	if env.Code == "" && len(factors) == 0 {
		return nil
	}
	return &ScaleSnapshot{
		ID:                   env.ID,
		Code:                 env.Code,
		ScaleVersion:         env.ScaleVersion,
		Title:                env.Title,
		QuestionnaireCode:    env.QuestionnaireCode,
		QuestionnaireVersion: env.QuestionnaireVersion,
		Status:               env.Status,
		Factors:              FactorsFromCanonical(factors),
	}
}
