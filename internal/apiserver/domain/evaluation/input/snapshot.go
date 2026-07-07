package input

// Answer is a single question response within an answer sheet.
type Answer struct {
	QuestionCode string
	Score        float64
	Value        any
}

// AnswerSheet captures submitted responses for one questionnaire version.
type AnswerSheet struct {
	QuestionnaireCode    string
	QuestionnaireVersion string
	Answers              []Answer
}

// Option is a selectable choice on a questionnaire question.
type Option struct {
	Code    string
	Content string
	Score   float64
}

// Question is a questionnaire item with scoring options.
type Question struct {
	Code    string
	Type    string
	Options []Option
}

// Questionnaire is the structural snapshot of a questionnaire version.
type Questionnaire struct {
	Code      string
	Version   string
	Title     string
	Questions []Question
}
