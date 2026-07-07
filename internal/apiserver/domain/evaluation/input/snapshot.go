package input

// Answer 是单一 题目作答 在 答卷。
type Answer struct {
	QuestionCode string
	Score        float64
	Value        any
}

// AnswerSheet 记录已提交作答 用于 一个问卷版本。
type AnswerSheet struct {
	QuestionnaireCode    string
	QuestionnaireVersion string
	Answers              []Answer
}

// Option 是selectable 选项 on 问卷 question。
type Option struct {
	Code    string
	Content string
	Score   float64
}

// Question 是问卷题目 使用 计分选项。
type Question struct {
	Code    string
	Type    string
	Options []Option
}

// Questionnaire 是structural 快照 of 问卷版本。
type Questionnaire struct {
	Code      string
	Version   string
	Title     string
	Questions []Question
}
