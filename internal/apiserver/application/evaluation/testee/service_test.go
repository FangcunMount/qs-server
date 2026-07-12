package testee

import (
	"context"
	"errors"
	"testing"
	"time"

	legacy "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/assessment"
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

type ownershipStub struct{ err error }

func (s ownershipStub) GetMine(context.Context, uint64, uint64) (*legacy.AssessmentResult, error) {
	if s.err != nil {
		return nil, s.err
	}
	return &legacy.AssessmentResult{ID: 1, TesteeID: 7}, nil
}

type scoreStub struct{ called bool }

func (s *scoreStub) GetByAssessmentID(context.Context, uint64) (*legacy.ScoreResult, error) {
	s.called = true
	return &legacy.ScoreResult{AssessmentID: 1}, nil
}
func (*scoreStub) GetFactorTrend(context.Context, legacy.GetFactorTrendDTO) (*legacy.FactorTrendResult, error) {
	return nil, nil
}
func (*scoreStub) GetHighRiskFactors(context.Context, uint64) (*legacy.HighRiskFactorsResult, error) {
	return nil, nil
}

func TestGetScoreRejectsNonOwnerBeforeReadingScore(t *testing.T) {
	t.Parallel()
	scores := &scoreStub{}
	svc := NewService(ownershipStub{err: errors.New("forbidden")}, nil, scores)
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
