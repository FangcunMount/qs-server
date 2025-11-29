package code

// apiserver: questionnaire errors (120xxx).
const (
// ErrQuestionnaireNotFound - 404: Questionnaire not found.
ErrQuestionnaireNotFound int = iota + 120001

// ErrQuestionnaireAlreadyExists - 400: Questionnaire already exists.
ErrQuestionnaireAlreadyExists

// ErrQuestionnaireArchived - 400: Questionnaire is archived.
ErrQuestionnaireArchived

// ErrQuestionnaireInvalidCode - 400: Invalid questionnaire code.
ErrQuestionnaireInvalidCode

// ErrQuestionnaireInvalidTitle - 400: Invalid questionnaire title.
ErrQuestionnaireInvalidTitle

// ErrQuestionnaireInvalidInput - 400: Invalid input for questionnaire.
ErrQuestionnaireInvalidInput

// ErrQuestionnaireInvalidStatus - 400: Invalid questionnaire status.
ErrQuestionnaireInvalidStatus

// ErrQuestionnaireInvalidQuestion - 400: Invalid question in questionnaire.
ErrQuestionnaireInvalidQuestion

// ErrQuestionnaireQuestionNotFound - 404: Question not found in questionnaire.
ErrQuestionnaireQuestionNotFound

// ErrQuestionnaireQuestionAlreadyExists - 400: Question already exists in questionnaire.
ErrQuestionnaireQuestionAlreadyExists

// ErrQuestionnaireQuestionBasicInfoInvalid - 400: Question basic info is invalid.
ErrQuestionnaireQuestionBasicInfoInvalid

// ErrQuestionnaireQuestionInvalid - 400: Question is invalid.
ErrQuestionnaireQuestionInvalid

// ErrStatusInvalid - 400: Invalid status transition.
ErrStatusInvalid

// ErrQuestionAlreadyExists - 400: Question already exists.
ErrQuestionAlreadyExists

// ErrOptionEmpty - 400: Option is empty.
ErrOptionEmpty
)

func init() {
	register(ErrQuestionnaireNotFound, 404, "Questionnaire not found")
	register(ErrQuestionnaireAlreadyExists, 400, "Questionnaire already exists")
	register(ErrQuestionnaireArchived, 400, "Questionnaire is archived")
	register(ErrQuestionnaireInvalidCode, 400, "Invalid questionnaire code")
	register(ErrQuestionnaireInvalidTitle, 400, "Invalid questionnaire title")
	register(ErrQuestionnaireInvalidInput, 400, "Invalid input for questionnaire")
	register(ErrQuestionnaireInvalidStatus, 400, "Invalid questionnaire status")
	register(ErrQuestionnaireInvalidQuestion, 400, "Invalid question in questionnaire")
	register(ErrQuestionnaireQuestionNotFound, 404, "Question not found in questionnaire")
	register(ErrQuestionnaireQuestionAlreadyExists, 400, "Question already exists in questionnaire")
	register(ErrQuestionnaireQuestionBasicInfoInvalid, 400, "Question basic info is invalid")
	register(ErrQuestionnaireQuestionInvalid, 400, "Question is invalid")
	register(ErrStatusInvalid, 400, "Invalid status transition")
	register(ErrQuestionAlreadyExists, 400, "Question already exists")
	register(ErrOptionEmpty, 400, "Option is empty")
}
