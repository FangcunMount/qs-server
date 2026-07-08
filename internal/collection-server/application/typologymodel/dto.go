package typologymodel

type PersonalityModelResponse struct {
	Code                 string                         `json:"code"`
	Version              string                         `json:"version"`
	Title                string                         `json:"title"`
	Algorithm            string                         `json:"algorithm"`
	Description          string                         `json:"description"`
	QuestionnaireCode    string                         `json:"questionnaire_code"`
	QuestionnaireVersion string                         `json:"questionnaire_version"`
	Status               string                         `json:"status"`
	QuestionCount        int32                          `json:"question_count"`
	DimensionOrder       []string                       `json:"dimension_order,omitempty"`
	Dimensions           []PersonalityDimensionResponse `json:"dimensions,omitempty"`
	Outcomes             []PersonalityOutcomeResponse   `json:"outcomes,omitempty"`
}

type PersonalityDimensionResponse struct {
	Code      string `json:"code"`
	Name      string `json:"name"`
	LeftPole  string `json:"left_pole,omitempty"`
	RightPole string `json:"right_pole,omitempty"`
}

type PersonalityOutcomeResponse struct {
	Code     string `json:"code"`
	Name     string `json:"name"`
	OneLiner string `json:"one_liner,omitempty"`
	ImageURL string `json:"image_url,omitempty"`
}

type PersonalityModelSummaryResponse struct {
	Code                 string `json:"code"`
	Version              string `json:"version"`
	Title                string `json:"title"`
	Algorithm            string `json:"algorithm"`
	Description          string `json:"description"`
	QuestionnaireCode    string `json:"questionnaire_code"`
	QuestionnaireVersion string `json:"questionnaire_version"`
	Status               string `json:"status"`
	QuestionCount        int32  `json:"question_count"`
}

type ListPersonalityModelsRequest struct {
	Page      int32  `form:"page"`
	PageSize  int32  `form:"page_size"`
	Algorithm string `form:"algorithm"`
}

type ListPersonalityModelsResponse struct {
	Models     []PersonalityModelSummaryResponse `json:"models"`
	Total      int64                             `json:"total"`
	Page       int32                             `json:"page"`
	PageSize   int32                             `json:"page_size"`
	TotalPages int32                             `json:"total_pages"`
}

type PersonalityModelCategoriesResponse struct {
	Categories []CategoryResponse `json:"categories"`
}

type CategoryResponse struct {
	Value string `json:"value"`
	Label string `json:"label"`
}
