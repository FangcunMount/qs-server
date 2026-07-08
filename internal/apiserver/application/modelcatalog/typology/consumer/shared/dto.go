package shared

// ListPersonalityModelsDTO 列出published 人格模型 用于 C 端 目录s。
type ListPersonalityModelsDTO struct {
	Page      int
	PageSize  int
	Algorithm string
}

// PersonalityModelSummaryResult 是list 题目 投影。
type PersonalityModelSummaryResult struct {
	Code                 string
	Version              string
	Title                string
	Algorithm            string
	Description          string
	QuestionnaireCode    string
	QuestionnaireVersion string
	Status               string
	QuestionCount        int
}

// PersonalityModelSummaryListResult 是paginated summary list。
type PersonalityModelSummaryListResult struct {
	Items      []PersonalityModelSummaryResult
	Total      int64
	Page       int
	PageSize   int
	TotalPages int
}

// PersonalityDimensionResult 暴露pole 元数据 用于 C 端 detail 视图。
type PersonalityDimensionResult struct {
	Code      string
	Name      string
	LeftPole  string
	RightPole string
}

// PersonalityOutcomeSummaryResult 暴露结果 cards 不使用 计分 内部s。
type PersonalityOutcomeSummaryResult struct {
	Code     string
	Name     string
	OneLiner string
	ImageURL string
}

// PersonalityModelResult 是detail 投影。
type PersonalityModelResult struct {
	PersonalityModelSummaryResult
	DimensionOrder []string
	Dimensions     []PersonalityDimensionResult
	Outcomes       []PersonalityOutcomeSummaryResult
}

// PersonalityModelCategoryResult 是算法/category 选项。
type PersonalityModelCategoryResult struct {
	Value string
	Label string
}

// PersonalityModelCategoriesResult 列出算法 选项。
type PersonalityModelCategoriesResult struct {
	Categories []PersonalityModelCategoryResult
}
