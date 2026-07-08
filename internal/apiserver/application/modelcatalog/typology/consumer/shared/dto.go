package shared

// ListTypologyModelsDTO 列出 published typology 模型，用于 C 端目录。
type ListTypologyModelsDTO struct {
	Page      int
	PageSize  int
	Algorithm string
}

// TypologyModelSummaryResult 是 list 条目投影。
type TypologyModelSummaryResult struct {
	Code                 string
	Version              string
	Title                string
	Algorithm            string
	Description          string
	QuestionnaireCode    string
	QuestionnaireVersion string
	Status               string
	QuestionCount        int
	Kind                 string
	SubKind              string
	ProductChannel       string
	AlgorithmFamily      string
	PayloadFormat        string
	DecisionKind         string
}

// TypologyModelSummaryListResult 是 paginated summary list。
type TypologyModelSummaryListResult struct {
	Items      []TypologyModelSummaryResult
	Total      int64
	Page       int
	PageSize   int
	TotalPages int
}

// TypologyDimensionResult 暴露 pole 元数据，用于 C 端 detail 视图。
type TypologyDimensionResult struct {
	Code      string
	Name      string
	LeftPole  string
	RightPole string
}

// TypologyOutcomeSummaryResult 暴露结果 cards，不使用计分内部结构。
type TypologyOutcomeSummaryResult struct {
	Code     string
	Name     string
	OneLiner string
	ImageURL string
}

// TypologyModelResult 是 detail 投影。
type TypologyModelResult struct {
	TypologyModelSummaryResult
	DimensionOrder []string
	Dimensions     []TypologyDimensionResult
	Outcomes       []TypologyOutcomeSummaryResult
}

// TypologyModelCategoryResult 是算法/category 选项。
type TypologyModelCategoryResult struct {
	Value string
	Label string
}

// TypologyModelCategoriesResult 列出算法选项。
type TypologyModelCategoriesResult struct {
	Categories []TypologyModelCategoryResult
}
