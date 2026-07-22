package statistics

import (
	"sort"
	"testing"
	"time"
)

func TestScanStableBatchesUsesOccurredAtAndIDCursor(t *testing.T) {
	type row struct {
		ID uint64
		At time.Time
	}
	from := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	to := from.Add(48 * time.Hour)
	source := make([]row, 0, 1003)
	for i := 0; i < 1003; i++ {
		at := from
		if i >= 600 {
			at = from.Add(24 * time.Hour)
		}
		source = append(source, row{ID: uint64(2000 - i), At: at})
	}
	sort.Slice(source, func(i, j int) bool {
		if source[i].At.Equal(source[j].At) {
			return source[i].ID < source[j].ID
		}
		return source[i].At.Before(source[j].At)
	})

	var page []row
	var collected []row
	err := scanStableBatches(from, &page, func(lastAt time.Time, lastID uint64) error {
		page = page[:0]
		for _, candidate := range source {
			if candidate.At.Before(from) || !candidate.At.Before(to) {
				continue
			}
			if candidate.At.After(lastAt) || (candidate.At.Equal(lastAt) && candidate.ID > lastID) {
				page = append(page, candidate)
				if len(page) == collectorBatchSize {
					break
				}
			}
		}
		return nil
	}, func(value row) (time.Time, uint64) {
		return value.At, value.ID
	}, func(batch []row) error {
		collected = append(collected, batch...)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(collected) != len(source) {
		t.Fatalf("collected=%d want %d", len(collected), len(source))
	}
	for i := range source {
		if collected[i] != source[i] {
			t.Fatalf("row %d=%+v want %+v", i, collected[i], source[i])
		}
	}
}

func TestTaskFactDoesNotBackfillLaterLifecycleIntoEarlierEvent(t *testing.T) {
	created := map[string]any{}
	applyTaskLifecycleFields(created, "task_created", nil)
	if _, exists := created["task_status"]; exists {
		t.Fatal("task_created must not copy a later current status")
	}
	if _, exists := created["completed_at"]; exists {
		t.Fatal("task_created must not copy a later completion time")
	}

	completedAt := time.Date(2026, 7, 20, 9, 30, 0, 0, time.FixedZone("CST", 8*3600))
	completed := map[string]any{}
	applyTaskLifecycleFields(completed, "task_completed", &completedAt)
	if completed["task_status"] != "completed" || completed["completed_at"] != &completedAt {
		t.Fatalf("completed fact=%v", completed)
	}
}
