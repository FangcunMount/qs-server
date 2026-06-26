package shared

// ListPersonalityModelsDTO lists published personality models for C-side catalogs.
type ListPersonalityModelsDTO struct {
	Page      int
	PageSize  int
	Algorithm string
}

// PersonalityModelSummaryResult is the list item projection.
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

// PersonalityModelSummaryListResult is a paginated summary list.
type PersonalityModelSummaryListResult struct {
	Items      []PersonalityModelSummaryResult
	Total      int64
	Page       int
	PageSize   int
	TotalPages int
}

// PersonalityDimensionResult exposes pole metadata for C-side detail views.
type PersonalityDimensionResult struct {
	Code      string
	Name      string
	LeftPole  string
	RightPole string
}

// PersonalityOutcomeSummaryResult exposes outcome cards without scoring internals.
type PersonalityOutcomeSummaryResult struct {
	Code     string
	Name     string
	OneLiner string
	ImageURL string
}

// PersonalityModelResult is the detail projection.
type PersonalityModelResult struct {
	PersonalityModelSummaryResult
	DimensionOrder []string
	Dimensions     []PersonalityDimensionResult
	Outcomes       []PersonalityOutcomeSummaryResult
}

// PersonalityModelCategoryResult is an algorithm/category option.
type PersonalityModelCategoryResult struct {
	Value string
	Label string
}

// PersonalityModelCategoriesResult lists algorithm options.
type PersonalityModelCategoriesResult struct {
	Categories []PersonalityModelCategoryResult
}
