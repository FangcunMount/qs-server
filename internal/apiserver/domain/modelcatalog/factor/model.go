package factor

// Factor 是 ModelCatalog 的通用测量节点。
type Factor struct {
	Code  string
	Title string
	Role  FactorRole
}

// ResolvedRole 返回显式 role，或归一化为空 role 的默认维度角色。
func (f Factor) ResolvedRole() FactorRole {
	return f.Role.Resolved()
}

// LegacyFactor 是历史 flat payload 的 compatibility materialization，不是核心领域模型。
type LegacyFactor struct {
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
func (f LegacyFactor) ResolvedRole() FactorRole {
	return resolveRole(f.Role, f.IsTotalScore)
}

// Snapshot 返回 LegacyFactor 的兼容 read/published 快照形态。
func (f LegacyFactor) Snapshot() FactorSnapshot {
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

// ScoringSourceKind 标识 Factor 计分输入来源。
type ScoringSourceKind string

const (
	ScoringSourceQuestion ScoringSourceKind = "question"
	ScoringSourceFactor   ScoringSourceKind = "factor"
)

// ScoringSource 指向一个计分输入，可以是题目或子 Factor。
type ScoringSource struct {
	Kind ScoringSourceKind
	Code string
}

// Scoring 描述一个 Factor 的分数如何由输入来源聚合得到。
type Scoring struct {
	FactorCode string
	Sources    []ScoringSource
	Strategy   ScoringStrategy
	Params     *ScoringParams
	MaxScore   *float64
	Weights    map[string]float64
}

// FactorEdge 描述 FactorGraph 中一条父子边。
type FactorEdge struct {
	ParentCode string
	ChildCode  string
}

// FactorGraph 描述 Factor 之间的层级和展示顺序。
type FactorGraph struct {
	Roots      []string
	Edges      []FactorEdge
	SortOrders map[string]int
}
