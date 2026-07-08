package typologymodel

type TypologyModelResponse struct {
	Code                 string                      `json:"code"`
	Version              string                      `json:"version"`
	Title                string                      `json:"title"`
	Algorithm            string                      `json:"algorithm"`
	Description          string                      `json:"description"`
	QuestionnaireCode    string                      `json:"questionnaire_code"`
	QuestionnaireVersion string                      `json:"questionnaire_version"`
	Status               string                      `json:"status"`
	QuestionCount        int32                       `json:"question_count"`
	Kind                 string                      `json:"kind,omitempty"`
	SubKind              string                      `json:"sub_kind,omitempty"`
	ProductChannel       string                      `json:"product_channel,omitempty"`
	AlgorithmFamily      string                      `json:"algorithm_family,omitempty"`
	PayloadFormat        string                      `json:"payload_format,omitempty"`
	DecisionKind         string                      `json:"decision_kind,omitempty"`
	DimensionOrder       []string                    `json:"dimension_order,omitempty"`
	Dimensions           []TypologyDimensionResponse `json:"dimensions,omitempty"`
	Outcomes             []TypologyOutcomeResponse   `json:"outcomes,omitempty"`
}

type TypologyDimensionResponse struct {
	Code      string `json:"code"`
	Name      string `json:"name"`
	LeftPole  string `json:"left_pole,omitempty"`
	RightPole string `json:"right_pole,omitempty"`
}

type TypologyOutcomeResponse struct {
	Code     string `json:"code"`
	Name     string `json:"name"`
	OneLiner string `json:"one_liner,omitempty"`
	ImageURL string `json:"image_url,omitempty"`
}

type TypologyModelSummaryResponse struct {
	Code                 string `json:"code"`
	Version              string `json:"version"`
	Title                string `json:"title"`
	Algorithm            string `json:"algorithm"`
	Description          string `json:"description"`
	QuestionnaireCode    string `json:"questionnaire_code"`
	QuestionnaireVersion string `json:"questionnaire_version"`
	Status               string `json:"status"`
	QuestionCount        int32  `json:"question_count"`
	Kind                 string `json:"kind,omitempty"`
	SubKind              string `json:"sub_kind,omitempty"`
	ProductChannel       string `json:"product_channel,omitempty"`
	AlgorithmFamily      string `json:"algorithm_family,omitempty"`
	PayloadFormat        string `json:"payload_format,omitempty"`
	DecisionKind         string `json:"decision_kind,omitempty"`
}

type ListTypologyModelsRequest struct {
	Page      int32  `form:"page"`
	PageSize  int32  `form:"page_size"`
	Algorithm string `form:"algorithm"`
}

type ListTypologyModelsResponse struct {
	Models     []TypologyModelSummaryResponse `json:"models"`
	Total      int64                          `json:"total"`
	Page       int32                          `json:"page"`
	PageSize   int32                          `json:"page_size"`
	TotalPages int32                          `json:"total_pages"`
}

type TypologyModelCategoriesResponse struct {
	Categories []TypologyCategoryResponse `json:"categories"`
}

type TypologyCategoryResponse struct {
	Value string `json:"value"`
	Label string `json:"label"`
}
