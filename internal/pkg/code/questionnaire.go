package code

// apiserver: questionnaire errors.
const (
	// ErrQuestionnaireNotFound - 404: Questionnaire not found.
	ErrQuestionnaireNotFound int = iota + 120001

	// ErrQuestionnaireAlreadyExists - 400: Questionnaire already exists.
	ErrQuestionnaireAlreadyExists

	// ErrQuestionnaireArchived - 400: Questionnaire is archived.
	ErrQuestionnaireArchived

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

	// ErrQuestionnaireStatusInvalid - 400: Invalid status transition.
	ErrQuestionnaireStatusInvalid
)
