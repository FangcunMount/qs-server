package factor

// Factor 是 ModelCatalog 的通用测量节点。
type Factor struct {
	Code            string
	Title           string
	Role            FactorRole
	ParentCode      string
	SortOrder       int
	Level           int
	IsTotalScore    bool
	QuestionCodes   []string
	ScoringStrategy string
	ScoringParams   *ScoringParams
	MaxScore        *float64
	InterpretRules  []ScoreRangeRule
	Classification  *ClassificationSpec
	Norm            *NormRef
	ChildrenPolicy  *ChildrenPolicy
}

// ResolvedRole 返回显式 role，或从旧版 total-score flag 推导 role。
func (f Factor) ResolvedRole() FactorRole {
	return resolveRole(f.Role, f.IsTotalScore)
}

// Snapshot 返回 Factor 的兼容 read/published 快照形态。
func (f Factor) Snapshot() FactorSnapshot {
	return FactorSnapshot{
		Code:            f.Code,
		Title:           f.Title,
		Role:            f.Role,
		ParentCode:      f.ParentCode,
		SortOrder:       f.SortOrder,
		Level:           f.Level,
		IsTotalScore:    f.IsTotalScore,
		QuestionCodes:   cloneStrings(f.QuestionCodes),
		ScoringStrategy: f.ScoringStrategy,
		ScoringParams:   cloneScoringParams(f.ScoringParams),
		MaxScore:        cloneFloat64(f.MaxScore),
		InterpretRules:  cloneScoreRangeRules(f.InterpretRules),
		Classification:  cloneClassificationSpec(f.Classification),
		Norm:            cloneNormRef(f.Norm),
		ChildrenPolicy:  cloneChildrenPolicy(f.ChildrenPolicy),
	}
}

// Scoring 描述一个 Factor 的题目分聚合策略。
type Scoring struct {
	FactorCode string
	Strategy   ScoringStrategy
	Params     *ScoringParams
}

// FactorGraph 描述 Factor 之间的层级和展示顺序。
type FactorGraph struct {
	Roots []string
	Edges map[string][]string
}
