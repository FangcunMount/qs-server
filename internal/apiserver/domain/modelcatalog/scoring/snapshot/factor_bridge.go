package snapshot

import "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"

// ExecutionEnvelope 携带non-因子 元数据 用于 scale-like execution。
type ExecutionEnvelope struct {
	ID                   uint64
	Code                 string
	ScaleVersion         string
	Title                string
	QuestionnaireCode    string
	QuestionnaireVersion string
	Status               string
}

// InterpretRuleFromScoreRange 投影规范 rule 为 scale execution 结构。
func InterpretRuleFromScoreRange(r factor.ScoreRangeRule) InterpretRuleSnapshot {
	return InterpretRuleSnapshot{
		Min:        r.MinScore,
		Max:        r.MaxScore,
		RiskLevel:  r.Level,
		Conclusion: r.Conclusion,
		Suggestion: r.Suggestion,
	}
}

// ScoreRangeFromInterpretRule 适配scale execution rule 为 规范 form。
func ScoreRangeFromInterpretRule(rule InterpretRuleSnapshot) factor.ScoreRangeRule {
	return factor.ScoreRangeRule{
		MinScore:   rule.Min,
		MaxScore:   rule.Max,
		Level:      rule.RiskLevel,
		Conclusion: rule.Conclusion,
		Suggestion: rule.Suggestion,
	}
}

// FactorFromCanonical projects a canonical snapshot into the scale execution shape.
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

// FactorFromDomainFactor projects a domain Factor into the scale execution shape.
func FactorFromDomainFactor(f factor.Factor) FactorSnapshot {
	return FactorFromCanonical(f.Snapshot())
}

// Canonical 适配scale execution 因子 为 规范 目录 form。
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

// FactorSnapshotFromCanonical 物化scale execution 因子 从 规范 form。
func FactorSnapshotFromCanonical(f factor.FactorSnapshot) FactorSnapshot {
	return FactorFromCanonical(f)
}

// FactorsFromCanonical 投影规范 因子 为 scale execution 因子。
func FactorsFromCanonical(factors []factor.FactorSnapshot) []FactorSnapshot {
	out := make([]FactorSnapshot, 0, len(factors))
	for _, item := range factors {
		out = append(out, FactorFromCanonical(item))
	}
	return out
}

// BuildFromModelFactors 物化scale 快照 从 common 家族 快照 元数据。
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

// BuildFromCanonicalFactors 物化scale execution 快照 从 规范 因子。
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
