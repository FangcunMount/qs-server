package main

import (
	"testing"
	"time"
)

func TestComputePlanFixupTimesPreservesExistingTTL(t *testing.T) {
	plannedAt := time.Date(2026, 4, 1, 9, 0, 0, 0, time.UTC)
	openAt := plannedAt.Add(2 * time.Hour)
	expireAt := openAt.Add(72 * time.Hour)

	times := computePlanFixupTimes(plannedAt, &openAt, &expireAt)
	if times.TTL != 72*time.Hour {
		t.Fatalf("expected ttl 72h, got %s", times.TTL)
	}
	if !times.OpenAt.Equal(plannedAt) {
		t.Fatalf("expected open_at %s, got %s", plannedAt, times.OpenAt)
	}
	if !times.ExpireAt.Equal(plannedAt.Add(72 * time.Hour)) {
		t.Fatalf("expected expire_at %s, got %s", plannedAt.Add(72*time.Hour), times.ExpireAt)
	}
	if !times.CompletionAt.Equal(plannedAt.Add(planFixupCompletionOffset)) {
		t.Fatalf("unexpected completion_at: %s", times.CompletionAt)
	}
	if !times.InterpretAt.Equal(plannedAt.Add(planFixupCompletionOffset + planFixupInterpretOffset)) {
		t.Fatalf("unexpected interpret_at: %s", times.InterpretAt)
	}
}

func TestComputePlanFixupTimesFallsBackToDefaultTTL(t *testing.T) {
	plannedAt := time.Date(2026, 4, 1, 9, 0, 0, 0, time.UTC)
	times := computePlanFixupTimes(plannedAt, nil, nil)
	if times.TTL != planFixupDefaultExpireTTL {
		t.Fatalf("expected default ttl %s, got %s", planFixupDefaultExpireTTL, times.TTL)
	}
}

func TestBuildPlanFixupTaskPatch(t *testing.T) {
	plannedAt := time.Date(2026, 4, 1, 9, 0, 0, 0, time.UTC)
	times := computePlanFixupTimes(plannedAt, nil, nil)

	completedPatch := buildPlanFixupTaskPatch("completed", times)
	if completedPatch.CompletedAt == nil || !completedPatch.CompletedAt.Equal(times.CompletionAt) {
		t.Fatalf("expected completed_at %s, got %+v", times.CompletionAt, completedPatch.CompletedAt)
	}
	if !completedPatch.UpdatedAt.Equal(times.CompletionAt) {
		t.Fatalf("expected completed updated_at %s, got %s", times.CompletionAt, completedPatch.UpdatedAt)
	}

	expiredPatch := buildPlanFixupTaskPatch("expired", times)
	if expiredPatch.CompletedAt != nil {
		t.Fatalf("expected expired completed_at nil, got %+v", expiredPatch.CompletedAt)
	}
	if !expiredPatch.UpdatedAt.Equal(times.ExpireAt) {
		t.Fatalf("expected expired updated_at %s, got %s", times.ExpireAt, expiredPatch.UpdatedAt)
	}

	openedPatch := buildPlanFixupTaskPatch("opened", times)
	if openedPatch.CompletedAt != nil {
		t.Fatalf("expected opened completed_at nil, got %+v", openedPatch.CompletedAt)
	}
	if !openedPatch.UpdatedAt.Equal(times.OpenAt) {
		t.Fatalf("expected opened updated_at %s, got %s", times.OpenAt, openedPatch.UpdatedAt)
	}
}

func TestParsePlanFixupScopeTesteeIDs(t *testing.T) {
	ids, err := parsePlanFixupScopeTesteeIDs([]string{"1001", " 1002 "})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 2 || ids[0] != 1001 || ids[1] != 1002 {
		t.Fatalf("unexpected parsed ids: %+v", ids)
	}

	if _, err := parsePlanFixupScopeTesteeIDs([]string{"bad-id"}); err == nil {
		t.Fatal("expected invalid id error")
	}
}

func TestComputePlanFixupTimesIsDeterministic(t *testing.T) {
	plannedAt := time.Date(2026, 4, 1, 9, 0, 0, 0, time.UTC)
	openAt := plannedAt
	expireAt := plannedAt.Add(24 * time.Hour)

	first := computePlanFixupTimes(plannedAt, &openAt, &expireAt)
	second := computePlanFixupTimes(plannedAt, &openAt, &expireAt)
	if first != second {
		t.Fatalf("expected deterministic fixup times, first=%+v second=%+v", first, second)
	}
}
