package survey

import (
	"testing"

	asApp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/answersheet"
	domainAnswerSheet "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	domainQuestionnaire "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/surveyreadmodel"
	"go.mongodb.org/mongo-driver/mongo"
)

func TestNormalizeDepsRequiresMongoDB(t *testing.T) {
	t.Parallel()

	if _, err := normalizeDeps(Deps{}); err == nil {
		t.Fatal("normalizeDeps() error = nil, want missing Mongo error")
	}
}

func TestNormalizeDepsDefaultsEventPublisher(t *testing.T) {
	t.Parallel()

	deps, err := normalizeDeps(minimalDeps())
	if err != nil {
		t.Fatalf("normalizeDeps() error = %v", err)
	}
	if deps.EventPublisher == nil {
		t.Fatal("EventPublisher = nil, want Nop publisher")
	}
}

func minimalDeps() Deps {
	return Deps{
		MongoDB:             &mongo.Database{},
		QuestionnaireRepo:   fakeQuestionnaireRepo{},
		QuestionnaireReader: fakeQuestionnaireReader{},
		AnswerSheetRepo:     fakeAnswerSheetStore{},
		AnswerSheetReader:   fakeAnswerSheetReader{},
	}
}

type fakeQuestionnaireRepo struct {
	domainQuestionnaire.Repository
}

type fakeQuestionnaireReader struct {
	surveyreadmodel.QuestionnaireReader
}

type fakeAnswerSheetStore struct {
	domainAnswerSheet.Repository
	asApp.SubmissionDurableWriter
}

type fakeAnswerSheetReader struct {
	surveyreadmodel.AnswerSheetReader
}
