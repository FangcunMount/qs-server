package main

import (
	"testing"
	"time"
)

func scheduleCandidateAt(status string) scheduleCandidate {
	createdAt := time.Date(2025, 1, 1, 19, 0, 0, 0, shanghai)
	return scheduleCandidate{
		ID:               42,
		Version:          3,
		ScheduleRevision: 1,
		Status:           status,
		PlannedAt:        createdAt.AddDate(0, 0, 7),
		SourceCreatedAt:  createdAt,
		CreatedAt:        createdAt,
		UpdatedAt:        createdAt.Add(time.Hour),
	}
}

func TestInferScheduleKeepsOriginalTaskAtRevisionOne(t *testing.T) {
	candidate := scheduleCandidateAt("pending")
	legacyPlanned := candidate.PlannedAt
	candidate.LegacyPlannedAt = &legacyPlanned

	got := inferSchedule(candidate)
	if got.Ambiguity != "" || got.Revision != 1 || !got.DefinedAt.Equal(candidate.SourceCreatedAt) || got.Reason != "original_schedule" {
		t.Fatalf("inference=%+v", got)
	}
}

func TestInferScheduleDetectsHistoricalPlannedAtChange(t *testing.T) {
	candidate := scheduleCandidateAt("pending")
	legacyPlanned := candidate.PlannedAt.AddDate(0, 0, -3)
	candidate.LegacyPlannedAt = &legacyPlanned
	candidate.UpdatedAt = candidate.SourceCreatedAt.Add(48 * time.Hour)

	got := inferSchedule(candidate)
	if got.Ambiguity != "" || got.Revision != 2 || !got.DefinedAt.Equal(candidate.UpdatedAt) || got.Reason != "collapsed_legacy_revisions" || got.Evidence != "planned_at_changed" {
		t.Fatalf("inference=%+v", got)
	}
}

func TestInferScheduleStartsRevisionTwoOneMillisecondAfterLegacyTerminal(t *testing.T) {
	candidate := scheduleCandidateAt("opened")
	legacyCanceled := candidate.SourceCreatedAt.Add(24 * time.Hour)
	openAt := legacyCanceled.Add(time.Hour)
	candidate.LegacyCanceled = &legacyCanceled
	candidate.OpenAt = &openAt

	got := inferSchedule(candidate)
	want := legacyCanceled.Add(time.Millisecond)
	if got.Ambiguity != "" || got.Revision != 2 || !got.DefinedAt.Equal(want) || got.Reason != "collapsed_legacy_revisions" || got.Evidence != "legacy_canceled_terminal" {
		t.Fatalf("inference=%+v want_defined=%s", got, want)
	}
}

func TestInferScheduleCollapsesMultipleLegacyTerminals(t *testing.T) {
	candidate := scheduleCandidateAt("pending")
	legacyExpired := candidate.SourceCreatedAt.Add(24 * time.Hour)
	legacyCanceled := legacyExpired.Add(24 * time.Hour)
	candidate.LegacyExpired = &legacyExpired
	candidate.LegacyCanceled = &legacyCanceled
	candidate.UpdatedAt = legacyCanceled.Add(time.Hour)

	got := inferSchedule(candidate)
	if got.Ambiguity != "" || got.Revision != 2 || got.Reason != "collapsed_legacy_revisions" || got.Evidence != "multiple_legacy_terminals" || !got.DefinedAt.Equal(legacyCanceled.Add(time.Millisecond)) {
		t.Fatalf("inference=%+v", got)
	}
}

func TestInferScheduleFailsClosedWhenLegacyAndCurrentLifecyclesOverlap(t *testing.T) {
	candidate := scheduleCandidateAt("opened")
	legacyCanceled := candidate.SourceCreatedAt.Add(24 * time.Hour)
	openAt := legacyCanceled.Add(time.Millisecond)
	candidate.LegacyCanceled = &legacyCanceled
	candidate.OpenAt = &openAt

	got := inferSchedule(candidate)
	if got.Ambiguity == "" || got.Revision != 2 {
		t.Fatalf("inference=%+v", got)
	}
}

func TestInferScheduleRejectsCompletedTaskThatWasLaterRestored(t *testing.T) {
	candidate := scheduleCandidateAt("pending")
	completedAt := candidate.SourceCreatedAt.Add(24 * time.Hour)
	candidate.LegacyCompleted = &completedAt

	got := inferSchedule(candidate)
	if got.Ambiguity == "" {
		t.Fatalf("inference=%+v", got)
	}
}

func TestInferScheduleRejectsDirtyPendingLifecycleFields(t *testing.T) {
	candidate := scheduleCandidateAt("pending")
	openAt := candidate.SourceCreatedAt.Add(time.Hour)
	candidate.OpenAt = &openAt

	got := inferSchedule(candidate)
	if got.Ambiguity == "" {
		t.Fatalf("inference=%+v", got)
	}
}
