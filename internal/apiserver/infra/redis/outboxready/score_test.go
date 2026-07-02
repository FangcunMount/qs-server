package outboxready

import (
	"testing"
	"time"
)

func TestReadyScoreOrdersByNextAttemptThenCreatedAt(t *testing.T) {
	due := time.Date(2026, 7, 2, 12, 0, 0, 500*int(time.Millisecond), time.UTC)
	older := due.Add(-2 * time.Minute)
	newer := due.Add(-1 * time.Minute)

	if got, want := ReadyScore(due, older), ReadyScore(due, newer); got >= want {
		t.Fatalf("older score = %v, newer = %v; want older < newer", got, want)
	}
	if got, want := ReadyScore(due.Add(-time.Second), older), ReadyScore(due, older); got >= want {
		t.Fatalf("earlier due score = %v, later due = %v; want earlier due < later due", got, want)
	}
}

func TestReadyScoreUsesCreatedAtWhenZero(t *testing.T) {
	due := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)
	if ReadyScore(due, time.Time{}) != ReadyScore(due, due) {
		t.Fatal("zero createdAt should fall back to nextAttemptAt")
	}
}
