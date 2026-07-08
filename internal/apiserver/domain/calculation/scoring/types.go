package scoring

import "github.com/FangcunMount/qs-server/internal/pkg/meta"

// Input is the neutral scale scoring input.
type Input struct {
	Model         Model
	AnswerSheet   *AnswerSheet
	Questionnaire *Questionnaire
}

type Model struct {
	Code                 string
	ScaleVersion         string
	Title                string
	QuestionnaireCode    string
	QuestionnaireVersion string
	Status               string
	Factors              []Factor
}

type Factor struct {
	Code            string
	Title           string
	ScoringStrategy string
	ScoringParams   CntParams
	QuestionCodes   []string
	MaxScore        *float64
	IsTotalScore    bool
	InterpretRules  []InterpretRule
}

type CntParams struct {
	CntOptionContents []string
}

type InterpretRule struct {
	Min        float64
	Max        float64
	RiskLevel  string
	Conclusion string
	Suggestion string
}

func (r InterpretRule) Matches(score float64) bool {
	return score >= r.Min && score < r.Max
}

type AnswerSheet struct {
	ID                   uint64
	QuestionnaireCode    string
	QuestionnaireVersion string
	Answers              []Answer
}

type Answer struct {
	QuestionCode meta.Code
	Score        float64
	Value        any
}

type Questionnaire struct {
	Code      string
	Version   string
	Questions []Question
}

type Question struct {
	Code    meta.Code
	Options []Option
}

type Option struct {
	Code    string
	Content string
	Score   float64
}

type Result struct {
	TotalScore   float64
	RiskLevel    RiskLevel
	FactorScores []FactorScore
}

type FactorScore struct {
	FactorCode   string
	FactorName   string
	RawScore     float64
	MaxScore     *float64
	RiskLevel    RiskLevel
	IsTotalScore bool
}
