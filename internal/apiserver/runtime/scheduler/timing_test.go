package scheduler

import (
	"testing"
	"time"
)

func TestParseDailyClock(t *testing.T) {
	clock, err := ParseDailyClock("00:30")
	if err != nil {
		t.Fatalf("ParseDailyClock returned error: %v", err)
	}
	if clock.Hour != 0 || clock.Minute != 30 {
		t.Fatalf("unexpected clock: %+v", clock)
	}
}

func TestNextDailyRun(t *testing.T) {
	now := time.Date(2026, 4, 17, 0, 29, 0, 0, time.Local)
	next := NextDailyRun(now, 0, 30)
	want := time.Date(2026, 4, 17, 0, 30, 0, 0, time.Local)
	if !next.Equal(want) {
		t.Fatalf("unexpected next run: got %s want %s", next, want)
	}

	now = time.Date(2026, 4, 17, 0, 31, 0, 0, time.Local)
	next = NextDailyRun(now, 0, 30)
	want = time.Date(2026, 4, 18, 0, 30, 0, 0, time.Local)
	if !next.Equal(want) {
		t.Fatalf("unexpected rolled next run: got %s want %s", next, want)
	}
}

func TestNextAlignedIntervalTickTime(t *testing.T) {
	loc := time.FixedZone("UTC+8", 8*60*60)
	tests := []struct {
		name     string
		now      time.Time
		interval time.Duration
		want     time.Time
	}{
		{
			name:     "one minute aligns to next minute boundary",
			now:      time.Date(2026, 4, 10, 18, 59, 50, 123000000, loc),
			interval: time.Minute,
			want:     time.Date(2026, 4, 10, 19, 0, 0, 0, loc),
		},
		{
			name:     "exact boundary advances by one interval",
			now:      time.Date(2026, 4, 10, 19, 0, 0, 0, loc),
			interval: time.Minute,
			want:     time.Date(2026, 4, 10, 19, 1, 0, 0, loc),
		},
		{
			name:     "five minutes aligns to local five minute boundary",
			now:      time.Date(2026, 4, 10, 18, 57, 12, 0, loc),
			interval: 5 * time.Minute,
			want:     time.Date(2026, 4, 10, 19, 0, 0, 0, loc),
		},
		{
			name:     "two hours rolls to next day boundary when needed",
			now:      time.Date(2026, 4, 10, 23, 59, 59, 0, loc),
			interval: 2 * time.Hour,
			want:     time.Date(2026, 4, 11, 0, 0, 0, 0, loc),
		},
		{
			name:     "non whole minute interval keeps relative cadence",
			now:      time.Date(2026, 4, 10, 18, 59, 50, 0, loc),
			interval: 90 * time.Second,
			want:     time.Date(2026, 4, 10, 19, 1, 20, 0, loc),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NextAlignedIntervalTickTime(tt.now, tt.interval)
			if !got.Equal(tt.want) {
				t.Fatalf("unexpected next tick time: got %s want %s", got.Format(time.RFC3339), tt.want.Format(time.RFC3339))
			}
		})
	}
}
