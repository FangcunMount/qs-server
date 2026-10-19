package survey

import (
	"context"
	"errors"
	"testing"

	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
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

type selectedSubmissionStore struct {
	reads int
	err   error
}

func (*selectedSubmissionStore) CreateDurably(context.Context, *domainAnswerSheet.AnswerSheet, asApp.DurableSubmitMeta) (*domainAnswerSheet.AnswerSheet, bool, error) {
	return nil, false, errors.New("fixture must not enter a new business transaction")
}
func (s *selectedSubmissionStore) FindCompleted(context.Context, asApp.DurableSubmitMeta) (*asApp.CompletedSubmission, error) {
	s.reads++
	return nil, s.err
}

type writeOnlySubmissionStore struct{}

func (writeOnlySubmissionStore) CreateDurably(context.Context, *domainAnswerSheet.AnswerSheet, asApp.DurableSubmitMeta) (*domainAnswerSheet.AnswerSheet, bool, error) {
	return nil, false, nil
}

func TestExplicitAnswerSubmissionStoreOwnsReadbackWithoutOutboxFallback(t *testing.T) {
	selected := &selectedSubmissionStore{err: errors.New("exact witness unresolved")}
	m := &Module{AnswerSheet: &AnswerSheetSubModule{}}
	// The actual module can use the explicitly selected original-transaction
	// store without constructing an Outbox stager or a post-commit Relay.
	err := m.initAnswerSheetSubModule(nil, nil, nil, nil, fakeAnswerSheetStore{}, fakeAnswerSheetReader{}, fakeQuestionnaireRepo{}, appEventing.ProfileBinding{}, selected)
	if err != nil {
		t.Fatal(err)
	}
	result, found, err := m.AnswerSheet.SubmissionService.LookupAcceptedSubmission(t.Context(), asApp.LookupSubmissionDTO{
		FillerID: 301, TesteeID: 401, IdempotencyKey: "same-operation", QuestionnaireCode: "M3", QuestionnaireVer: "1.0.0",
		Answers: []asApp.AnswerDTO{{QuestionCode: "Q1", QuestionType: "Text", Value: "answer"}},
	})
	if selected.reads != 1 || err == nil || result != nil || found {
		t.Fatal("unresolved selected-store witness was accepted or routed to legacy persistence")
	}
}

func TestExplicitAnswerSubmissionStoreRequiresAuthoritativeReader(t *testing.T) {
	m := &Module{AnswerSheet: &AnswerSheetSubModule{}}
	err := m.initAnswerSheetSubModule(nil, nil, nil, nil, fakeAnswerSheetStore{}, fakeAnswerSheetReader{}, fakeQuestionnaireRepo{}, appEventing.ProfileBinding{}, writeOnlySubmissionStore{})
	if err == nil {
		t.Fatal("write-only transaction store cannot support exact accepted-result recovery")
	}
}

func TestLegacyAnswerSubmissionStillRequiresCompleteOutboxProfile(t *testing.T) {
	m := &Module{AnswerSheet: &AnswerSheetSubModule{}}
	err := m.initAnswerSheetSubModule(nil, nil, nil, nil, fakeAnswerSheetStore{}, fakeAnswerSheetReader{}, fakeQuestionnaireRepo{}, appEventing.ProfileBinding{}, nil)
	if err == nil {
		t.Fatal("unconfigured legacy path silently admitted business")
	}
}
