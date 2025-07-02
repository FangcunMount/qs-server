package code

// apiserver: user errors.
const (
	// ErrUserNotFound - 404: User not found.
	ErrUserNotFound int = iota + 110001

	// ErrUserAlreadyExists- 400: User already exist.
	ErrUserAlreadyExists

	// ErrUserBasicInfoInvalid - 400: User basic info is invalid.
	ErrUserBasicInfoInvalid

	// ErrUserStatusInvalid - 400: User status is invalid.
	ErrUserStatusInvalid

	// ErrUserInvalid - 400: User is invalid.
	ErrUserInvalid

	// ErrUserBlocked - 403: User is blocked.
	ErrUserBlocked

	// ErrUserInactive - 403: User is inactive.
	ErrUserInactive
)

// apiserver: questionnaire errors.
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
)
