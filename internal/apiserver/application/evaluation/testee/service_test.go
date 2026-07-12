package testee

import (
	"context"
	"testing"
	"time"

	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	domaintestee "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	domainassessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestParseDate(t *testing.T) {
	t.Parallel()
	got, err := parseDate("2026-04-22", true)
	if err != nil || got == nil || got.Format("2006-01-02") != "2026-04-23" {
		t.Fatalf("unexpected end-exclusive date: %v, %v", got, err)
	}
	got, err = parseDate(time.Date(2026, 4, 22, 8, 0, 0, 0, time.UTC).Format(time.RFC3339), false)
	if err != nil || got == nil {
		t.Fatalf("expected RFC3339 date to parse, got %v, %v", got, err)
	}
	if _, err := parseDate("bad-date", false); err == nil {
		t.Fatal("expected invalid date error")
	}
}

type assessmentRepoStub struct {
	domainassessment.Repository
	value *domainassessment.Assessment
}

func (s assessmentRepoStub) FindByID(context.Context, domainassessment.ID) (*domainassessment.Assessment, error) {
	return s.value, nil
}

type scoreStub struct{ called bool }

func (s *scoreStub) Get(context.Context, uint64) (*evaloutcome.ScoreFact, error) {
	s.called = true
	return &evaloutcome.ScoreFact{AssessmentID: 1}, nil
}
func (*scoreStub) Trend(context.Context, uint64, string, int) (*evaloutcome.FactorTrendFact, error) {
	return nil, nil
}

func TestGetScoreRejectsNonOwnerBeforeReadingScore(t *testing.T) {
	t.Parallel()
	scores := &scoreStub{}
	a, err := domainassessment.NewAssessment(9, domaintestee.NewID(7), domainassessment.NewQuestionnaireRefByCode(meta.NewCode("Q"), "1"), domainassessment.NewAnswerSheetRef(meta.FromUint64(2)), domainassessment.NewAdhocOrigin(), domainassessment.WithID(meta.FromUint64(1)))
	if err != nil {
		t.Fatal(err)
	}
	svc := NewService(assessmentRepoStub{value: a}, nil, scores)
	if _, err := svc.GetScore(context.Background(), Actor{TesteeID: 8}, 1); err == nil {
		t.Fatal("expected ownership error")
	}
	if scores.called {
		t.Fatal("score reader called before ownership was established")
	}
}

func TestNormalizeStatusesDoesNotReintroduceInterpreted(t *testing.T) {
	t.Parallel()
	got := normalizeStatuses("done")
	if len(got) != 1 || got[0] != "evaluated" {
		t.Fatalf("done statuses = %v, want evaluated only", got)
	}
}
