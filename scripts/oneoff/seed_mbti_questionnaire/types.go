package main

type questionnaireSeedFile struct {
	Code        string         `json:"code"`
	Version     string         `json:"version"`
	Title       string         `json:"title"`
	Description string         `json:"description"`
	ImgURL      string         `json:"img_url"`
	Type        string         `json:"type"`
	Questions   []questionSeed `json:"questions"`
}

type questionSeed struct {
	Code     string       `json:"code"`
	Stem     string       `json:"stem"`
	Type     string       `json:"type"`
	Required bool         `json:"required"`
	Options  []optionSeed `json:"options"`
}

type optionSeed struct {
	Code    string  `json:"code"`
	Content string  `json:"content"`
	Score   float64 `json:"score"`
}

type mbtiModelSeedFile struct {
	QuestionnaireVersion string `json:"questionnaire_version"`
	QuestionMappings     []struct {
		QuestionCode string  `json:"question_code"`
		Dimension    string  `json:"dimension"`
		Sign         float64 `json:"sign"`
	} `json:"question_mappings"`
}
