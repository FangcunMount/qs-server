package questionnaire

import (
	"testing"

	domainQuestionnaire "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestQuestionnaireDomainErrorCodeMapsDomainKindsToAPICodes(t *testing.T) {
	t.Parallel()

	_, err := domainQuestionnaire.NewQuestionnaire(meta.NewCode(""), "")
	if err == nil {
		t.Fatal("expected domain error")
	}
	if got := questionnaireDomainErrorCode(err, errorCode.ErrUnknown); got != errorCode.ErrQuestionnaireInvalidCode {
		t.Fatalf("mapped code = %d, want %d", got, errorCode.ErrQuestionnaireInvalidCode)
	}
}
