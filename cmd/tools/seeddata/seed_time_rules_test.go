package main

import (
	"testing"
	"time"
)

func TestDeriveRelationBoundAt(t *testing.T) {
	base := time.Date(2026, 4, 1, 8, 0, 0, 0, time.Local)

	cases := []struct {
		name         string
		relationType string
		want         time.Time
	}{
		{
			name:         "primary",
			relationType: "primary",
			want:         base.Add(2 * time.Hour),
		},
		{
			name:         "attending alias assigned",
			relationType: "assigned",
			want:         base.Add(4 * time.Hour),
		},
		{
			name:         "collaborator",
			relationType: "collaborator",
			want:         base.Add(6 * time.Hour),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := deriveRelationBoundAt(base, tc.relationType)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !got.Equal(tc.want) {
				t.Fatalf("unexpected bound_at: got=%v want=%v", got, tc.want)
			}
		})
	}
}

func TestDeriveEntryTimeline(t *testing.T) {
	entryCreatedAt := time.Date(2026, 4, 2, 9, 0, 0, 0, time.Local)
	testeeCreatedAt := time.Date(2026, 4, 1, 10, 0, 0, 0, time.Local)

	resolveAt := deriveEntryResolveAt(entryCreatedAt, testeeCreatedAt)
	wantResolveAt := testeeCreatedAt.Add(24 * time.Hour)
	if !resolveAt.Equal(wantResolveAt) {
		t.Fatalf("unexpected resolve_at: got=%v want=%v", resolveAt, wantResolveAt)
	}

	intakeAt := deriveEntryIntakeAt(resolveAt)
	if !intakeAt.Equal(resolveAt.Add(10 * time.Minute)) {
		t.Fatalf("unexpected intake_at: got=%v want=%v", intakeAt, resolveAt.Add(10*time.Minute))
	}

	relationAt := deriveEntryAccessRelationAt(intakeAt)
	if !relationAt.Equal(intakeAt.Add(time.Minute)) {
		t.Fatalf("unexpected entry relation time: got=%v want=%v", relationAt, intakeAt.Add(time.Minute))
	}

	submittedAt := deriveEntryAssessmentSubmitAt(intakeAt)
	if !submittedAt.Equal(intakeAt.Add(20 * time.Minute)) {
		t.Fatalf("unexpected assessment submit time: got=%v want=%v", submittedAt, intakeAt.Add(20*time.Minute))
	}

	interpretedAt := deriveAssessmentInterpretAt(submittedAt)
	if !interpretedAt.Equal(submittedAt.Add(30 * time.Second)) {
		t.Fatalf("unexpected interpreted time: got=%v want=%v", interpretedAt, submittedAt.Add(30*time.Second))
	}
}

func TestDeriveActorCreatedAt(t *testing.T) {
	firstBoundAt := time.Date(2026, 4, 3, 12, 0, 0, 0, time.Local)

	clinicianCreatedAt := deriveClinicianCreatedAt(firstBoundAt)
	wantClinicianCreatedAt := firstBoundAt.Add(-7 * 24 * time.Hour)
	if !clinicianCreatedAt.Equal(wantClinicianCreatedAt) {
		t.Fatalf("unexpected clinician created_at: got=%v want=%v", clinicianCreatedAt, wantClinicianCreatedAt)
	}

	staffCreatedAt := deriveStaffCreatedAt(clinicianCreatedAt)
	wantStaffCreatedAt := clinicianCreatedAt.Add(-24 * time.Hour)
	if !staffCreatedAt.Equal(wantStaffCreatedAt) {
		t.Fatalf("unexpected staff created_at: got=%v want=%v", staffCreatedAt, wantStaffCreatedAt)
	}
}

func TestNormalizeEntryLimits(t *testing.T) {
	if got := normalizeMaxIntakesPerEntry(0); got != seedEntryFlowDefaultMaxIntakes {
		t.Fatalf("unexpected default max intakes: got=%d want=%d", got, seedEntryFlowDefaultMaxIntakes)
	}
	if got := normalizeMaxAssessmentsPerEntry(-1); got != seedByEntryDefaultMaxCount {
		t.Fatalf("unexpected default max assessments: got=%d want=%d", got, seedByEntryDefaultMaxCount)
	}
}

func TestActorWaveAllocatorProducesWavePattern(t *testing.T) {
	schedule := actorWaveSchedule{
		WaveInterval: 90 * 24 * time.Hour,
		WaveWeeks:    1,
		DayStartHour: 9,
		DayEndHour:   10,
		SlotInterval: 30 * time.Minute,
		WaveDays:     []time.Weekday{time.Monday},
	}
	base := time.Date(2026, 4, 7, 8, 0, 0, 0, time.Local) // Tuesday
	allocator := newActorWaveAllocator(base, schedule)

	first := allocator.NextAtOrAfter(base)
	second := allocator.NextAtOrAfter(base)
	third := allocator.NextAtOrAfter(base)

	if first.Weekday() != time.Monday || first.Hour() != 9 || first.Minute() != 0 {
		t.Fatalf("unexpected first slot: %v", first)
	}
	if second.Weekday() != time.Monday || second.Hour() != 9 || second.Minute() != 30 {
		t.Fatalf("unexpected second slot: %v", second)
	}
	if third.Sub(second) < 80*24*time.Hour {
		t.Fatalf("expected third slot to jump to next wave, got gap=%v third=%v", third.Sub(second), third)
	}
	if third.Weekday() != time.Monday || third.Hour() != 9 || third.Minute() != 0 {
		t.Fatalf("unexpected third slot: %v", third)
	}
}
