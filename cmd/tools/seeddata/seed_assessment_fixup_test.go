package main

import (
	"testing"
	"time"
)

func TestAssessmentMatchesEntryTarget(t *testing.T) {
	t.Run("scale target matches medical scale code", func(t *testing.T) {
		scaleCode := "3adyDE"
		if !assessmentMatchesEntryTarget(assessmentFixupAssessmentRow{
			MedicalScaleCode: &scaleCode,
		}, assessmentFixupEntryRow{
			TargetType: "scale",
			TargetCode: "3adyDE",
		}) {
			t.Fatalf("expected scale target to match medical_scale_code")
		}
	})

	t.Run("questionnaire target with explicit version", func(t *testing.T) {
		version := "v2"
		if !assessmentMatchesEntryTarget(assessmentFixupAssessmentRow{
			QuestionnaireCode:    "Q001",
			QuestionnaireVersion: "v2",
		}, assessmentFixupEntryRow{
			TargetType:    "questionnaire",
			TargetCode:    "Q001",
			TargetVersion: &version,
		}) {
			t.Fatalf("expected questionnaire target to match exact version")
		}
		if assessmentMatchesEntryTarget(assessmentFixupAssessmentRow{
			QuestionnaireCode:    "Q001",
			QuestionnaireVersion: "v1",
		}, assessmentFixupEntryRow{
			TargetType:    "questionnaire",
			TargetCode:    "Q001",
			TargetVersion: &version,
		}) {
			t.Fatalf("expected questionnaire target version mismatch to fail")
		}
	})

	t.Run("questionnaire target without version accepts same code", func(t *testing.T) {
		if !assessmentMatchesEntryTarget(assessmentFixupAssessmentRow{
			QuestionnaireCode:    "Q001",
			QuestionnaireVersion: "v9",
		}, assessmentFixupEntryRow{
			TargetType: "questionnaire",
			TargetCode: "Q001",
		}) {
			t.Fatalf("expected questionnaire target without version to match by code")
		}
	})
}

func TestDeriveStandaloneAssessmentSubmitTimes(t *testing.T) {
	createdAt := time.Date(2024, 1, 2, 9, 0, 0, 0, time.Local)
	ceiling := deriveStandaloneAssessmentSubmitCeiling(createdAt)
	rows := []assessmentFixupAssessmentRow{
		{ID: 101, TesteeID: 1},
		{ID: 102, TesteeID: 1},
		{ID: 103, TesteeID: 1},
	}

	targets := deriveStandaloneAssessmentSubmitTimes(createdAt, rows, ceiling)
	if len(targets) != len(rows) {
		t.Fatalf("unexpected target count: got %d want %d", len(targets), len(rows))
	}

	firstExpected := createdAt.Add(assessmentFixupStandaloneInitialOffset)
	if !targets[0].Equal(firstExpected) {
		t.Fatalf("unexpected first submit time: got %s want %s", targets[0], firstExpected)
	}
	for idx := 1; idx < len(targets); idx++ {
		if targets[idx].Before(targets[idx-1]) {
			t.Fatalf("submit times must be non-decreasing: idx=%d prev=%s current=%s", idx, targets[idx-1], targets[idx])
		}
		if targets[idx].After(ceiling) {
			t.Fatalf("submit time exceeded ceiling: idx=%d current=%s ceiling=%s", idx, targets[idx], ceiling)
		}
	}
}

func TestDeriveStandaloneAssessmentSubmitTimesCompressesLateHistory(t *testing.T) {
	createdAt := time.Date(2026, 4, 14, 22, 0, 0, 0, time.Local)
	ceiling := deriveStandaloneAssessmentSubmitCeiling(createdAt)
	rows := []assessmentFixupAssessmentRow{
		{ID: 201, TesteeID: 2},
		{ID: 202, TesteeID: 2},
		{ID: 203, TesteeID: 2},
	}

	targets := deriveStandaloneAssessmentSubmitTimes(createdAt, rows, ceiling)
	if len(targets) != len(rows) {
		t.Fatalf("unexpected target count: got %d want %d", len(targets), len(rows))
	}
	for idx := range targets {
		if targets[idx].After(ceiling) {
			t.Fatalf("compressed submit time exceeded ceiling: idx=%d current=%s ceiling=%s", idx, targets[idx], ceiling)
		}
		if idx > 0 && targets[idx].Before(targets[idx-1]) {
			t.Fatalf("compressed submit times must be non-decreasing: idx=%d prev=%s current=%s", idx, targets[idx-1], targets[idx])
		}
	}
}

func TestDeriveStandaloneAssessmentSubmitCeiling(t *testing.T) {
	t.Run("uses testee created_at plus thirty days for normal history", func(t *testing.T) {
		createdAt := time.Date(2024, 1, 2, 9, 0, 0, 0, time.Local)
		got := deriveStandaloneAssessmentSubmitCeiling(createdAt)
		want := createdAt.Round(0).Add(assessmentFixupStandaloneWindow - seedAssessmentInterpretOffset)
		if !got.Equal(want) {
			t.Fatalf("unexpected ceiling: got=%s want=%s", got, want)
		}
	})

	t.Run("caps near the global historical range end", func(t *testing.T) {
		createdAt := time.Date(2026, 4, 14, 22, 0, 0, 0, time.Local)
		got := deriveStandaloneAssessmentSubmitCeiling(createdAt)
		want := testeeCreatedAtFixupRangeEnd.Add(-seedAssessmentInterpretOffset).Round(0)
		if !got.Equal(want) {
			t.Fatalf("unexpected capped ceiling: got=%s want=%s", got, want)
		}
	})
}

func TestParseAssessmentFixupInterpretedAtScope(t *testing.T) {
	t.Run("date-only upper bound expands to next day exclusive", func(t *testing.T) {
		scope, err := parseAssessmentFixupInterpretedAtScope("2026-03-01", "2026-04-16")
		if err != nil {
			t.Fatalf("unexpected parse error: %v", err)
		}
		if scope.From == nil || scope.To == nil {
			t.Fatalf("expected both range bounds to be set, got %+v", scope)
		}
		if !scope.ToExclusive {
			t.Fatalf("expected date-only upper bound to be exclusive, got %+v", scope)
		}
		wantFrom := time.Date(2026, 3, 1, 0, 0, 0, 0, time.Local)
		wantTo := time.Date(2026, 4, 17, 0, 0, 0, 0, time.Local)
		if !scope.From.Equal(wantFrom) {
			t.Fatalf("unexpected from bound: got=%s want=%s", scope.From.Format(time.RFC3339), wantFrom.Format(time.RFC3339))
		}
		if !scope.To.Equal(wantTo) {
			t.Fatalf("unexpected to bound: got=%s want=%s", scope.To.Format(time.RFC3339), wantTo.Format(time.RFC3339))
		}
	})

	t.Run("datetime upper bound remains inclusive", func(t *testing.T) {
		scope, err := parseAssessmentFixupInterpretedAtScope("2026-03-01 08:00:00", "2026-04-16 12:34:56")
		if err != nil {
			t.Fatalf("unexpected parse error: %v", err)
		}
		if scope.From == nil || scope.To == nil {
			t.Fatalf("expected both range bounds to be set, got %+v", scope)
		}
		if scope.ToExclusive {
			t.Fatalf("expected datetime upper bound to remain inclusive, got %+v", scope)
		}
		wantTo := time.Date(2026, 4, 16, 12, 34, 56, 0, time.Local)
		if !scope.To.Equal(wantTo) {
			t.Fatalf("unexpected inclusive to bound: got=%s want=%s", scope.To.Format(time.RFC3339), wantTo.Format(time.RFC3339))
		}
	})

	t.Run("invalid inverted range fails", func(t *testing.T) {
		_, err := parseAssessmentFixupInterpretedAtScope("2026-04-16", "2026-03-01")
		if err == nil {
			t.Fatal("expected inverted range to fail")
		}
	})
}
