package answersheet

import "time"

const (
	idempotencyStatusCompleted = "completed"
)

// AnswerSheetSubmitIdempotencyPO stores the durable result of an idempotent
// answersheet submission request.
type AnswerSheetSubmitIdempotencyPO struct {
	IdempotencyKey       string    `bson:"idempotency_key"`
	WriterID             uint64    `bson:"writer_id"`
	TesteeID             uint64    `bson:"testee_id"`
	QuestionnaireCode    string    `bson:"questionnaire_code"`
	QuestionnaireVersion string    `bson:"questionnaire_version"`
	AnswerSheetID        uint64    `bson:"answersheet_id"`
	Status               string    `bson:"status"`
	ErrorMessage         string    `bson:"error_message,omitempty"`
	CreatedAt            time.Time `bson:"created_at"`
	UpdatedAt            time.Time `bson:"updated_at"`
}

func (AnswerSheetSubmitIdempotencyPO) CollectionName() string {
	return "answersheet_submit_idempotency"
}
