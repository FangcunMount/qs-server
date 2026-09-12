package answersheet

import "context"

type DurableSubmitResultReader interface {
	LookupAcceptedSubmission(ctx context.Context, input *LookupAcceptedSubmissionInput) (*LookupAcceptedSubmissionOutput, error)
}

type LookupAcceptedSubmissionInput struct {
	AnsweringStartID     uint64
	QuestionnaireCode    string
	QuestionnaireVersion string
	IdempotencyKey       string
	WriterID             uint64
	TesteeID             uint64
	TaskID               string
	OriginRef            *OriginRef
	Answers              []AnswerInput
}

type LookupAcceptedSubmissionOutput struct {
	Found bool
	ID    uint64
}
