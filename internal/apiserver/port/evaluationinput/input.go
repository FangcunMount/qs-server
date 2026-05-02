package evaluationinput

import "context"

type InputRef struct {
	AssessmentID         uint64
	MedicalScaleCode     string
	AnswerSheetID        uint64
	QuestionnaireCode    string
	QuestionnaireVersion string
}

type InputSnapshot struct {
	MedicalScale  *ScaleSnapshot
	AnswerSheet   *AnswerSheetSnapshot
	Questionnaire *QuestionnaireSnapshot
}

type ScaleSnapshot struct {
	ID                   uint64
	Code                 string
	Title                string
	QuestionnaireCode    string
	QuestionnaireVersion string
	Status               string
	Factors              []FactorSnapshot
}

func (s *ScaleSnapshot) IsPublished() bool {
	return s != nil && s.Status == "published"
}

func (s *ScaleSnapshot) FindFactor(code string) (*FactorSnapshot, bool) {
	if s == nil {
		return nil, false
	}
	for i := range s.Factors {
		if s.Factors[i].Code == code {
			return &s.Factors[i], true
		}
	}
	return nil, false
}

type FactorSnapshot struct {
	Code            string
	Title           string
	IsTotalScore    bool
	QuestionCodes   []string
	ScoringStrategy string
	ScoringParams   ScoringParamsSnapshot
	MaxScore        *float64
	InterpretRules  []InterpretRuleSnapshot
}

func (f FactorSnapshot) QuestionCount() int {
	return len(f.QuestionCodes)
}

func (f FactorSnapshot) FindInterpretRule(score float64) *InterpretRuleSnapshot {
	for i := range f.InterpretRules {
		if f.InterpretRules[i].Matches(score) {
			return &f.InterpretRules[i]
		}
	}
	return nil
}

type ScoringParamsSnapshot struct {
	CntOptionContents []string
}

type InterpretRuleSnapshot struct {
	Min        float64
	Max        float64
	RiskLevel  string
	Conclusion string
	Suggestion string
}

func (r InterpretRuleSnapshot) Matches(score float64) bool {
	return score >= r.Min && score < r.Max
}

type AnswerSheetSnapshot struct {
	ID                   uint64
	QuestionnaireCode    string
	QuestionnaireVersion string
	QuestionnaireTitle   string
	Answers              []AnswerSnapshot
}

type AnswerSnapshot struct {
	QuestionCode string
	Score        float64
	Value        any
}

type QuestionnaireSnapshot struct {
	Code      string
	Version   string
	Title     string
	Questions []QuestionSnapshot
}

type QuestionSnapshot struct {
	Code    string
	Type    string
	Options []OptionSnapshot
}

type OptionSnapshot struct {
	Code    string
	Content string
	Score   float64
}

type Resolver interface {
	Resolve(ctx context.Context, ref InputRef) (*InputSnapshot, error)
}

type ScaleCatalog interface {
	GetScale(ctx context.Context, code string) (*ScaleSnapshot, error)
}

type FailureReasonCarrier interface {
	FailureReason() string
}
