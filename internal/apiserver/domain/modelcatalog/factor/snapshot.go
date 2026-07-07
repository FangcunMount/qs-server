package factor

// FactorSnapshot 是规范 published-model 维度 definition。
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
	if f.Role != "" {
		return f.Role
	}
	if f.IsTotalScore {
		return FactorRoleTotal
	}
	return FactorRoleDimension
}
