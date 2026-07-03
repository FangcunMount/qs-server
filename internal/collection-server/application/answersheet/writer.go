package answersheet

import "context"

// AnswerSheetWriter 是答卷提交用例依赖的写端口，屏蔽下游 gRPC DTO。
type AnswerSheetWriter interface {
	SaveAnswerSheet(ctx context.Context, input *SaveAnswerSheetInput) (*SaveAnswerSheetOutput, error)
}

// SaveAnswerSheetInput 是 collection application 层的答卷保存输入。
type SaveAnswerSheetInput struct {
	QuestionnaireCode    string
	QuestionnaireVersion string
	IdempotencyKey       string
	Title                string
	WriterID             uint64
	TesteeID             uint64
	TaskID               string
	OrgID                uint64
	Answers              []AnswerInput
}

// AnswerInput 是 collection application 层的答案保存输入。
type AnswerInput struct {
	QuestionCode string
	QuestionType string
	Score        uint32
	Value        string
}

// SaveAnswerSheetOutput 是 collection application 层的答卷保存结果。
type SaveAnswerSheetOutput struct {
	ID      uint64
	Message string
}
