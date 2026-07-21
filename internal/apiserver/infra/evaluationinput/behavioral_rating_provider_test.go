package evaluationinput_test

import (
	"context"
	"testing"
	"time"

	evaluationinputInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/evaluationinput"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	behavioralpayload "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/behavioral"
)

type stubNormSubjectReader struct {
	facts *port.NormSubjectFacts
	err   error
}

func (s stubNormSubjectReader) ReadNormSubjectFacts(context.Context, uint64) (*port.NormSubjectFacts, error) {
	return s.facts, s.err
}

type stubBehavioralCatalog struct{}

func (stubBehavioralCatalog) GetBehavioralRatingByRef(context.Context, port.ModelRef) (*behavioralpayload.Snapshot, error) {
	return &behavioralpayload.Snapshot{Code: "BRIEF2", Version: "1.0.0", Title: "BRIEF-2"}, nil
}

func (stubBehavioralCatalog) FindBehavioralRatingByQuestionnaire(context.Context, string, string) (*behavioralpayload.Snapshot, error) {
	return nil, nil
}

type stubAnswerSheetReader struct{}

func (stubAnswerSheetReader) GetAnswerSheet(context.Context, uint64) (*port.AnswerSheetSnapshot, error) {
	return &port.AnswerSheetSnapshot{ID: 3, QuestionnaireCode: "Q", QuestionnaireVersion: "1"}, nil
}

type stubQuestionnaireReader struct{}

func (stubQuestionnaireReader) GetQuestionnaire(context.Context, string, string) (*port.QuestionnaireSnapshot, error) {
	return &port.QuestionnaireSnapshot{Code: "Q", Version: "1"}, nil
}

func TestBehavioralRatingProviderAttachesNormSubject(t *testing.T) {
	birthday := time.Date(2018, 1, 1, 0, 0, 0, 0, time.UTC)
	asOf := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	provider := evaluationinputInfra.NewBehavioralRatingModelInputProvider(
		"brief2",
		stubBehavioralCatalog{},
		nil,
		stubAnswerSheetReader{},
		stubQuestionnaireReader{},
		stubNormSubjectReader{facts: &port.NormSubjectFacts{Gender: "male", Birthday: &birthday}},
	)

	snapshot, err := provider.ResolveInput(context.Background(), port.InputRef{
		AnswerSheetID: 3,
		TesteeID:      7,
		AsOf:          asOf,
		ModelRef:      port.ModelRef{Algorithm: "brief2", Code: "BRIEF2", Version: "1.0.0"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.NormSubject == nil || snapshot.NormSubject.AgeMonths == nil || *snapshot.NormSubject.AgeMonths != 72 || snapshot.NormSubject.Gender != "male" {
		t.Fatalf("NormSubject = %#v", snapshot.NormSubject)
	}
}
