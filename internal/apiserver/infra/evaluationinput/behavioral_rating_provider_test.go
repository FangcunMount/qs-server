package evaluationinput_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	evaluationinputInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/evaluationinput"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	rulesetport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
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

type failingPublishedModelReader struct {
	err error
}

func (s failingPublishedModelReader) GetPublishedModelByRef(context.Context, rulesetport.Ref) (*rulesetport.PublishedModel, error) {
	return nil, s.err
}

func (s failingPublishedModelReader) FindPublishedModelByQuestionnaire(context.Context, string, string) (*rulesetport.PublishedModel, error) {
	return nil, s.err
}

func requireRetryableDependency(t *testing.T, err error, wantCategory port.DependencyCategory) {
	t.Helper()
	if err == nil {
		t.Fatal("expected retryable dependency error")
	}
	var kind port.FailureKindCarrier
	if !errors.As(err, &kind) || kind.FailureKind() != port.FailureKindDependencyUnavailable {
		t.Fatalf("error = %T %v, want dependency_unavailable", err, err)
	}
	var retryable port.RetryableCarrier
	if !errors.As(err, &retryable) || !retryable.Retryable() {
		t.Fatalf("error = %T %v, want retryable carrier", err, err)
	}
	var category port.DependencyCategoryCarrier
	if !errors.As(err, &category) {
		t.Fatalf("error = %T %v, want dependency category %q", err, err, wantCategory)
	}
	if category.DependencyCategory() != wantCategory {
		t.Fatalf("dependency category = %q, want %q", category.DependencyCategory(), wantCategory)
	}
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

func TestBehavioralRatingProviderClassifiesNormSubjectReaderFailureAsRetryableActorDependency(t *testing.T) {
	provider := evaluationinputInfra.NewBehavioralRatingModelInputProvider(
		modelcatalog.AlgorithmBrief2,
		stubBehavioralCatalog{},
		nil,
		stubAnswerSheetReader{},
		stubQuestionnaireReader{},
		stubNormSubjectReader{err: errors.New("actor timeout")},
	)

	_, err := provider.ResolveInput(context.Background(), port.InputRef{
		AnswerSheetID: 3,
		TesteeID:      7,
		AsOf:          time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		ModelRef: port.ModelRef{
			Kind:      port.EvaluationModelKindBehavioralRating,
			Algorithm: string(modelcatalog.AlgorithmBrief2),
			Code:      "BRIEF2",
			Version:   "1.0.0",
		},
	})
	requireRetryableDependency(t, err, port.DependencyCategoryActor)
}

func TestBehavioralRatingProviderClassifiesCanonicalReloadFailureAsRetryableModelCatalogDependency(t *testing.T) {
	provider := evaluationinputInfra.NewBehavioralRatingModelInputProvider(
		modelcatalog.AlgorithmBrief2,
		stubBehavioralCatalog{},
		failingPublishedModelReader{err: errors.New("model catalog timeout")},
		stubAnswerSheetReader{},
		stubQuestionnaireReader{},
		nil,
	)

	_, err := provider.ResolveInput(context.Background(), port.InputRef{
		AnswerSheetID: 3,
		TesteeID:      7,
		AsOf:          time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		ModelRef: port.ModelRef{
			Kind:      port.EvaluationModelKindBehavioralRating,
			Algorithm: string(modelcatalog.AlgorithmBrief2),
			Code:      "BRIEF2",
			Version:   "1.0.0",
		},
	})
	requireRetryableDependency(t, err, port.DependencyCategoryModelCatalog)
}
