package assembler

import (
	"testing"

	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	asApp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/answersheet"
	domainAnswerSheet "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	domainQuestionnaire "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/surveyreadmodel"
	"go.mongodb.org/mongo-driver/mongo"
)

func TestNormalizeSurveyModuleDepsRequiresMongoDB(t *testing.T) {
	t.Parallel()

	if _, err := normalizeSurveyModuleDeps(SurveyModuleDeps{}); err == nil {
		t.Fatal("normalizeSurveyModuleDeps() error = nil, want missing Mongo error")
	}
}

func TestNormalizeSurveyModuleDepsDefaultsEventPublisher(t *testing.T) {
	t.Parallel()

	deps, err := normalizeSurveyModuleDeps(minimalSurveyModuleDeps())
	if err != nil {
		t.Fatalf("normalizeSurveyModuleDeps() error = %v", err)
	}
	if deps.EventPublisher == nil {
		t.Fatal("EventPublisher = nil, want Nop publisher")
	}
}

func minimalSurveyModuleDeps() SurveyModuleDeps {
	return SurveyModuleDeps{
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
	asApp.EventStager
	asApp.SubmittedEventOutboxStore
	appEventing.OutboxStatusReader
}

type fakeAnswerSheetReader struct {
	surveyreadmodel.AnswerSheetReader
}
