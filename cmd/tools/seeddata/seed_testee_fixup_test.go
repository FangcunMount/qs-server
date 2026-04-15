package main

import (
	"testing"
	"time"
)

func TestBuildTesteeCreatedAtYearBuckets_PreservesBoundaries(t *testing.T) {
	start := time.Date(2019, 3, 25, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 4, 15, 23, 59, 59, 0, time.UTC)

	buckets, err := buildTesteeCreatedAtYearBuckets(start, end, testeeCreatedAtFixupYearWeights)
	if err != nil {
		t.Fatalf("buildTesteeCreatedAtYearBuckets returned error: %v", err)
	}
	if len(buckets) != 8 {
		t.Fatalf("expected 8 buckets, got %d", len(buckets))
	}
	if !buckets[0].Start.Equal(start) {
		t.Fatalf("expected first bucket start %s, got %s", start.Format(time.RFC3339), buckets[0].Start.Format(time.RFC3339))
	}
	if !buckets[0].End.Equal(time.Date(2019, 12, 31, 23, 59, 59, 0, time.UTC)) {
		t.Fatalf("unexpected first bucket end: %s", buckets[0].End.Format(time.RFC3339))
	}
	last := buckets[len(buckets)-1]
	if !last.Start.Equal(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("unexpected last bucket start: %s", last.Start.Format(time.RFC3339))
	}
	if !last.End.Equal(end) {
		t.Fatalf("expected last bucket end %s, got %s", end.Format(time.RFC3339), last.End.Format(time.RFC3339))
	}
}

func TestAllocateTesteeCreatedAtCounts_NormalizesWeights(t *testing.T) {
	start := time.Date(2019, 3, 25, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 4, 15, 23, 59, 59, 0, time.UTC)
	buckets, err := buildTesteeCreatedAtYearBuckets(start, end, testeeCreatedAtFixupYearWeights)
	if err != nil {
		t.Fatalf("buildTesteeCreatedAtYearBuckets returned error: %v", err)
	}

	counts, err := allocateTesteeCreatedAtCounts(102, buckets)
	if err != nil {
		t.Fatalf("allocateTesteeCreatedAtCounts returned error: %v", err)
	}

	want := map[int]int{
		2019: 5,
		2020: 6,
		2021: 11,
		2022: 18,
		2023: 22,
		2024: 25,
		2025: 13,
		2026: 2,
	}
	for year, expected := range want {
		if got := counts[year]; got != expected {
			t.Fatalf("expected year %d count %d, got %d", year, expected, got)
		}
	}
}

func TestBuildWeightedTesteeCreatedAtTargets_PreservesGlobalBoundaries(t *testing.T) {
	start := time.Date(2019, 3, 25, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 4, 15, 23, 59, 59, 0, time.UTC)

	targets, allocation, err := buildWeightedTesteeCreatedAtTargets(102, start, end, testeeCreatedAtFixupYearWeights)
	if err != nil {
		t.Fatalf("buildWeightedTesteeCreatedAtTargets returned error: %v", err)
	}
	if len(targets) != 102 {
		t.Fatalf("expected 102 targets, got %d", len(targets))
	}
	if !targets[0].Equal(start) {
		t.Fatalf("expected first target %s, got %s", start.Format(time.RFC3339), targets[0].Format(time.RFC3339))
	}
	if !targets[len(targets)-1].Equal(end) {
		t.Fatalf("expected last target %s, got %s", end.Format(time.RFC3339), targets[len(targets)-1].Format(time.RFC3339))
	}
	if allocation[2019] != 5 || allocation[2026] != 2 {
		t.Fatalf("unexpected allocation edges: %+v", allocation)
	}
}

func TestDeriveEvenlyDistributedTimestamp_UsesMidpointForMiddleItem(t *testing.T) {
	start := time.Date(2021, 5, 18, 0, 0, 0, 0, time.UTC)
	end := start.Add(10 * time.Hour)

	got, err := deriveEvenlyDistributedTimestamp(1, 3, start, end)
	if err != nil {
		t.Fatalf("deriveEvenlyDistributedTimestamp returned error: %v", err)
	}

	want := start.Add(5 * time.Hour)
	if !got.Equal(want) {
		t.Fatalf("expected midpoint %s, got %s", want.Format(time.RFC3339), got.Format(time.RFC3339))
	}
}
