package factor

// FactorSnapshot 是兼容 read/published DTO，不是 Factor 核心领域模型。
// 新领域逻辑优先使用 Factor；runtime、legacy payload adapter 可继续使用该快照形态。
type FactorSnapshot struct {
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

// ResolvedRole 返回显式 角色 或 derives 一个from 旧版 flags。
func (f FactorSnapshot) ResolvedRole() FactorRole {
	return resolveRole(f.Role, f.IsTotalScore)
}

// Factor materializes the domain Factor represented by this compatibility snapshot.
func (f FactorSnapshot) Factor() Factor {
	return FactorFromSnapshot(f)
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
