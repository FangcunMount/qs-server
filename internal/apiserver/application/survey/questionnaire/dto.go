package questionnaire

// ============= DTO 定义 =============
// DTOs 用于应用服务层的输入参数

// CreateQuestionnaireDTO 创建问卷 DTO
type CreateQuestionnaireDTO struct {
	Code        string // 问卷编码（可选，用于导入/种子）
	Title       string // 问卷标题
	Description string // 问卷描述
	ImgUrl      string // 封面图URL
	Version     string // 初始版本（可选）
}

// UpdateQuestionnaireBasicInfoDTO 更新问卷基本信息 DTO
type UpdateQuestionnaireBasicInfoDTO struct {
	Code        string // 问卷编码
	Title       string // 问卷标题
	Description string // 问卷描述
	ImgUrl      string // 封面图URL
}

// AddQuestionDTO 添加问题 DTO
type AddQuestionDTO struct {
	QuestionnaireCode string      // 问卷编码
	Code              string      // 问题编码
	Stem              string      // 题干
	Type              string      // 问题类型
	Options           []OptionDTO // 选项列表
	Required          bool        // 是否必填
	Description       string      // 问题描述
}

// UpdateQuestionDTO 更新问题 DTO
type UpdateQuestionDTO struct {
	QuestionnaireCode string      // 问卷编码
	Code              string      // 问题编码
	Stem              string      // 题干
	Type              string      // 问题类型
	Options           []OptionDTO // 选项列表
	Required          bool        // 是否必填
	Description       string      // 问题描述
}

// OptionDTO 选项 DTO
type OptionDTO struct {
	Label string // 选项标签
	Value string // 选项值
	Score int    // 选项分数
}

// QuestionDTO 问题 DTO（用于批量更新）
type QuestionDTO struct {
	Code        string      // 问题编码
	Stem        string      // 题干
	Type        string      // 问题类型
	Options     []OptionDTO // 选项列表
	Required    bool        // 是否必填
	Description string      // 问题描述
}

// ListQuestionnairesDTO 查询问卷列表 DTO
type ListQuestionnairesDTO struct {
	Page       int               // 页码
	PageSize   int               // 每页数量
	Conditions map[string]string // 查询条件
}
