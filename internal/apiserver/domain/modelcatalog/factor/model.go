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

func resolveRole(role FactorRole, isTotalScore bool) FactorRole {
	if role != "" {
		return role
	}
	if isTotalScore {
		return FactorRoleTotal
	}
	return FactorRoleDimension
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
