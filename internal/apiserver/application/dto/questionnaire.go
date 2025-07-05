package dto

// QuestionnaireDTO 问卷数据传输对象
type QuestionnaireDTO struct {
	ID          uint64        `json:"id"`
	Code        string        `json:"code"`
	Title       string        `json:"title"`
	Description string        `json:"description"`
	ImgUrl      string        `json:"img_url"`
	Version     string        `json:"version"`
	Status      string        `json:"status"`
	Questions   []QuestionDTO `json:"questions"`
}

// QuestionnaireListDTO 问卷列表数据传输对象
type QuestionnaireListDTO struct {
	Total          int64               `json:"total"`
	Questionnaires []*QuestionnaireDTO `json:"questionnaires"`
}

// QuestionDTO 用于 application 层问题组合结构
type QuestionDTO struct {
	Code        string      // 问题编码
	Title       string      // 问题标题
	Type        string      // 问题类型
	Tips        string      // 问题提示
	Placeholder string      // 占位符（用于文本类型问题）
	Options     []OptionDTO // 选项列表

	// 验证规则
	ValidationRules []ValidationRuleDTO // 验证规则列表

	// 计算规则
	CalculationRule *CalculationRuleDTO // 计算规则
}

// OptionDTO 用于 application 层选项组合结构
type OptionDTO struct {
	Code    string // 选项编码
	Content string // 选项内容
	Score   int    // 选项分值
}

// ValidationRuleDTO 验证规则 DTO
type ValidationRuleDTO struct {
	RuleType    string // 规则类型
	TargetValue string // 目标值
}

// CalculationRuleDTO 计算规则 DTO
type CalculationRuleDTO struct {
	FormulaType string // 公式类型
}
