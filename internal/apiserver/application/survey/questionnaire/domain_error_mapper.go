package questionnaire

import (
	"github.com/FangcunMount/component-base/pkg/errors"
	domainQuestionnaire "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
)

func wrapQuestionnaireDomainError(err error, fallbackCode int, format string, args ...interface{}) error {
	if err == nil {
		return nil
	}
	return errors.WrapC(err, questionnaireDomainErrorCode(err, fallbackCode), format, args...)
}

func questionnaireDomainErrorCode(err error, fallbackCode int) int {
	kind, ok := domainQuestionnaire.ErrorKindOf(err)
	if !ok {
		return fallbackCode
	}
	switch kind {
	case domainQuestionnaire.ErrorKindInvalidCode:
		return errorCode.ErrQuestionnaireInvalidCode
	case domainQuestionnaire.ErrorKindInvalidTitle:
		return errorCode.ErrQuestionnaireInvalidTitle
	case domainQuestionnaire.ErrorKindInvalidInput:
		return errorCode.ErrQuestionnaireInvalidInput
	case domainQuestionnaire.ErrorKindInvalidQuestion:
		return errorCode.ErrQuestionnaireInvalidQuestion
	case domainQuestionnaire.ErrorKindQuestionExists:
		return errorCode.ErrQuestionAlreadyExists
	case domainQuestionnaire.ErrorKindQuestionNotFound:
		return errorCode.ErrQuestionnaireQuestionNotFound
	case domainQuestionnaire.ErrorKindArchived:
		return errorCode.ErrQuestionnaireArchived
	case domainQuestionnaire.ErrorKindInvalidStatus:
		return errorCode.ErrQuestionnaireInvalidStatus
	case domainQuestionnaire.ErrorKindOptionEmpty:
		return errorCode.ErrOptionEmpty
	default:
		return fallbackCode
	}
}
