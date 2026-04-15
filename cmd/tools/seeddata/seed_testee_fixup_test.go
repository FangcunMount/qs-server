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
	rows := make([]testeeCreatedAtFixupRow, 0, 102)
	for idx := 0; idx < 102; idx++ {
		rows = append(rows, testeeCreatedAtFixupRow{ID: uint64(idx + 1)})
	}

	targets, allocation, err := buildWeightedTesteeCreatedAtTargets(rows, start, end, testeeCreatedAtFixupYearWeights)
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

func TestDeriveDeterministicBucketTimestamps_IsStableAndJittered(t *testing.T) {
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2024, 12, 31, 23, 59, 59, 0, time.UTC)
	rows := []testeeCreatedAtFixupRow{
		{ID: 101},
		{ID: 102},
		{ID: 103},
		{ID: 104},
		{ID: 105},
	}

	firstRun, err := deriveDeterministicBucketTimestamps(2024, rows, start, end)
	if err != nil {
		t.Fatalf("deriveDeterministicBucketTimestamps returned error: %v", err)
	}
	secondRun, err := deriveDeterministicBucketTimestamps(2024, rows, start, end)
	if err != nil {
		t.Fatalf("deriveDeterministicBucketTimestamps returned error on second run: %v", err)
	}

	if len(firstRun) != len(rows) {
		t.Fatalf("expected %d timestamps, got %d", len(rows), len(firstRun))
	}
	for idx := range firstRun {
		if !firstRun[idx].Equal(secondRun[idx]) {
			t.Fatalf("expected deterministic timestamp at index %d, got %s and %s", idx, firstRun[idx].Format(time.RFC3339), secondRun[idx].Format(time.RFC3339))
		}
		if idx > 0 && firstRun[idx].Before(firstRun[idx-1]) {
			t.Fatalf("expected non-decreasing timestamps, got %s before %s", firstRun[idx].Format(time.RFC3339), firstRun[idx-1].Format(time.RFC3339))
		}
	}
	if !firstRun[0].Equal(start) {
		t.Fatalf("expected first timestamp at bucket start %s, got %s", start.Format(time.RFC3339), firstRun[0].Format(time.RFC3339))
	}
	if !firstRun[len(firstRun)-1].Equal(end) {
		t.Fatalf("expected last timestamp at bucket end %s, got %s", end.Format(time.RFC3339), firstRun[len(firstRun)-1].Format(time.RFC3339))
	}

	evenMidpoint, err := deriveEvenlyDistributedTimestamp(2, len(rows), start, end)
	if err != nil {
		t.Fatalf("deriveEvenlyDistributedTimestamp returned error: %v", err)
	}
	if firstRun[2].Equal(evenMidpoint) {
		t.Fatalf("expected jittered middle timestamp to differ from evenly distributed midpoint %s", evenMidpoint.Format(time.RFC3339))
	}
}

func TestDeriveDeterministicBucketTimestamps_PrefersWeekdaysAndAddsDailyVolatility(t *testing.T) {
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2024, 3, 31, 23, 59, 59, 0, time.UTC)
	rows := make([]testeeCreatedAtFixupRow, 0, 900)
	for idx := 0; idx < 900; idx++ {
		rows = append(rows, testeeCreatedAtFixupRow{ID: uint64(1000 + idx)})
	}

	targets, err := deriveDeterministicBucketTimestamps(2024, rows, start, end)
	if err != nil {
		t.Fatalf("deriveDeterministicBucketTimestamps returned error: %v", err)
	}

	weekdayTotal := 0
	weekendTotal := 0
	weekdayDays := 0
	weekendDays := 0
	weekdayCounts := make(map[int]int)
	dailyCounts := make(map[string]int)
	for _, ts := range targets {
		dayKey := ts.Format("2006-01-02")
		dailyCounts[dayKey]++
	}
	for day := start; !day.After(end); day = day.Add(24 * time.Hour) {
		count := dailyCounts[day.Format("2006-01-02")]
		if isTesteeCreatedAtWeekday(day.Weekday()) {
			weekdayTotal += count
			weekdayDays++
			weekdayCounts[count]++
		} else {
			weekendTotal += count
			weekendDays++
		}
	}
	if weekdayDays == 0 || weekendDays == 0 {
		t.Fatalf("expected both weekday and weekend days in range")
	}

	avgWeekday := float64(weekdayTotal) / float64(weekdayDays)
	avgWeekend := float64(weekendTotal) / float64(weekendDays)
	if avgWeekday <= avgWeekend*1.7 {
		t.Fatalf("expected weekday load to be significantly higher than weekend load, got weekday %.2f weekend %.2f", avgWeekday, avgWeekend)
	}
	if len(weekdayCounts) < 4 {
		t.Fatalf("expected visible weekday volatility, got only %d distinct weekday daily counts", len(weekdayCounts))
	}
}

func TestAllocateTesteeCreatedAtDayCounts_UsesAllRows(t *testing.T) {
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2024, 1, 14, 23, 59, 59, 0, time.UTC)
	slots, err := buildTesteeCreatedAtDaySlots(2024, start, end)
	if err != nil {
		t.Fatalf("buildTesteeCreatedAtDaySlots returned error: %v", err)
	}

	counts, err := allocateTesteeCreatedAtDayCounts(140, slots)
	if err != nil {
		t.Fatalf("allocateTesteeCreatedAtDayCounts returned error: %v", err)
	}
	if len(counts) != len(slots) {
		t.Fatalf("expected %d day counts, got %d", len(slots), len(counts))
	}

	total := 0
	for _, count := range counts {
		total += count
	}
	if total != 140 {
		t.Fatalf("expected day counts to sum to 140, got %d", total)
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
