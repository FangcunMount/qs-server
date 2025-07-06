package code

// answersheet errors.
const (
	// ErrAnswerSheetNotFound - 404: Answer sheet not found.
	ErrAnswerSheetNotFound int = iota + 110001

	// ErrAnswerNotFound - 404: Answer not found.
	ErrAnswerNotFound

	// ErrAnswerSheetInvalid - 400: Answer sheet is invalid.
	ErrAnswerSheetInvalid
)
