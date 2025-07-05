package code

// questionnaire errors.
const (
	// ErrQuestionnaireNotFound - 404: Questionnaire not found.
	ErrQuestionnaireNotFound int = iota + 110001

	// ErrQuestionnaireAlreadyExists- 400: Questionnaire already exist.
	ErrQuestionnaireAlreadyExists

	// ErrQuestionnaireBasicInfoInvalid - 400: Questionnaire basic info is invalid.
	ErrQuestionnaireBasicInfoInvalid

	// ErrQuestionnaireStatusInvalid - 400: Questionnaire status is invalid.
	ErrQuestionnaireStatusInvalid

	// ErrQuestionnaireQuestionInvalid - 400: Questionnaire question is invalid.
	ErrQuestionnaireQuestionInvalid

	// ErrQuestionnaireQuestionNotFound - 404: Questionnaire question not found.
	ErrQuestionnaireQuestionNotFound

	// ErrQuestionnaireQuestionAlreadyExists - 400: Questionnaire question already exist.
	ErrQuestionnaireQuestionAlreadyExists

	// ErrQuestionnaireQuestionBasicInfoInvalid - 400: Questionnaire question basic info is invalid.
	ErrQuestionnaireQuestionBasicInfoInvalid

	// ErrQuestionnairePublished - 400: Questionnaire is published, can't edit.
	ErrQuestionnairePublished

	// ErrQuestionnaireArchived - 400: Questionnaire is archived, can't edit.
	ErrQuestionnaireArchived
)
