package scale

// ============= DTO 定义 =============
// DTOs 用于应用服务层的输入参数

// CreateScaleDTO 创建量表 DTO
type CreateScaleDTO struct {
	Title                string // 量表标题
	Description          string // 量表描述
	QuestionnaireCode    string // 关联的问卷编码
	QuestionnaireVersion string // 关联的问卷版本
}

// UpdateScaleBasicInfoDTO 更新量表基本信息 DTO
type UpdateScaleBasicInfoDTO struct {
	Code        string // 量表编码
	Title       string // 量表标题
	Description string // 量表描述
}

// UpdateScaleQuestionnaireDTO 更新量表关联问卷 DTO
type UpdateScaleQuestionnaireDTO struct {
	Code                 string // 量表编码
	QuestionnaireCode    string // 关联的问卷编码
	QuestionnaireVersion string // 关联的问卷版本
}

// AddFactorDTO 添加因子 DTO
type AddFactorDTO struct {
	ScaleCode       string             // 量表编码
	Code            string             // 因子编码
	Title           string             // 因子标题
	FactorType      string             // 因子类型：primary/multilevel
	IsTotalScore    bool               // 是否为总分因子
	QuestionCodes   []string           // 关联的题目编码列表
	ScoringStrategy string             // 计分策略：sum/avg/custom
	ScoringParams   map[string]string  // 计分参数
	InterpretRules  []InterpretRuleDTO // 解读规则列表
}

// UpdateFactorDTO 更新因子 DTO
type UpdateFactorDTO struct {
	ScaleCode       string             // 量表编码
	Code            string             // 因子编码
	Title           string             // 因子标题
	FactorType      string             // 因子类型
	IsTotalScore    bool               // 是否为总分因子
	QuestionCodes   []string           // 关联的题目编码列表
	ScoringStrategy string             // 计分策略
	ScoringParams   map[string]string  // 计分参数
	InterpretRules  []InterpretRuleDTO // 解读规则列表
}

// FactorDTO 因子 DTO（用于批量替换）
type FactorDTO struct {
	Code            string             // 因子编码
	Title           string             // 因子标题
	FactorType      string             // 因子类型
	IsTotalScore    bool               // 是否为总分因子
	QuestionCodes   []string           // 关联的题目编码列表
	ScoringStrategy string             // 计分策略
	ScoringParams   map[string]string  // 计分参数
	InterpretRules  []InterpretRuleDTO // 解读规则列表
}

// InterpretRuleDTO 解读规则 DTO
type InterpretRuleDTO struct {
	MinScore   float64 // 最小分数（含）
	MaxScore   float64 // 最大分数（不含）
	RiskLevel  string  // 风险等级：none/low/medium/high/severe
	Conclusion string  // 结论文案
	Suggestion string  // 建议文案
}

// UpdateFactorInterpretRulesDTO 更新因子解读规则 DTO
type UpdateFactorInterpretRulesDTO struct {
	ScaleCode      string             // 量表编码
	FactorCode     string             // 因子编码
	InterpretRules []InterpretRuleDTO // 解读规则列表
}

// ListScalesDTO 查询量表列表 DTO
type ListScalesDTO struct {
	Page       int               // 页码
	PageSize   int               // 每页数量
	Conditions map[string]string // 查询条件
}
