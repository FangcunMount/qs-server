package questionnaire

import "errors"

// 领域业务规则错误
var (
	ErrEmptyTitle                         = errors.New("questionnaire title cannot be empty")
	ErrCannotModifyPublishedQuestionnaire = errors.New("cannot modify published questionnaire")
	ErrCannotPublishEmptyQuestionnaire    = errors.New("cannot publish questionnaire without questions")
	ErrAlreadyPublished                   = errors.New("questionnaire is already published")
	ErrAlreadyArchived                    = errors.New("questionnaire is already archived")
	ErrQuestionnaireNotFound              = errors.New("questionnaire not found")
	ErrDuplicateCode                      = errors.New("questionnaire code already exists")
	ErrQuestionNotFound                   = errors.New("question not found")
)
