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
	ceiling := time.Date(2024, 12, 31, 23, 59, 59, 0, time.Local)
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
	ceiling := time.Date(2026, 4, 15, 23, 59, 59, 0, time.Local)
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
