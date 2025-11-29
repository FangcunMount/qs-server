package code

// answersheet errors (111xxx).
const (
// ErrAnswerSheetNotFound - 404: Answer sheet not found.
ErrAnswerSheetNotFound int = iota + 111001

// ErrAnswerNotFound - 404: Answer not found.
ErrAnswerNotFound

// ErrAnswerSheetInvalid - 400: Answer sheet is invalid.
ErrAnswerSheetInvalid
)

func init() {
	register(ErrAnswerSheetNotFound, 404, "Answer sheet not found")
	register(ErrAnswerNotFound, 404, "Answer not found")
	register(ErrAnswerSheetInvalid, 400, "Answer sheet is invalid")
}
