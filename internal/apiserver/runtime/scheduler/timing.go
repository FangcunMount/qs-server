package scheduler

import (
	"context"
	"time"
)

// DailyClock is a parsed wall-clock schedule.
type DailyClock struct {
	Hour   int
	Minute int
}

// ParseDailyClock parses a daily HH:MM wall-clock time in local timezone.
func ParseDailyClock(raw string) (DailyClock, error) {
	parsed, err := time.ParseInLocation("15:04", raw, time.Local)
	if err != nil {
		return DailyClock{}, err
	}
	return DailyClock{Hour: parsed.Hour(), Minute: parsed.Minute()}, nil
}

// NextDailyRun returns the next local wall-clock execution time.
func NextDailyRun(now time.Time, hour, minute int) time.Time {
	next := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())
	if !next.After(now) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}

// WaitDelay waits for the given delay or returns early when the context is cancelled.
func WaitDelay(ctx context.Context, delay time.Duration) bool {
	if delay <= 0 {
		return ctx.Err() == nil
	}

	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// WaitUntilNextAlignedInterval waits until the next aligned interval boundary.
func WaitUntilNextAlignedInterval(ctx context.Context, interval time.Duration) bool {
	nextTickAt := NextAlignedIntervalTickTime(time.Now(), interval)
	return WaitDelay(ctx, time.Until(nextTickAt))
}

// NextAlignedIntervalTickTime aligns whole-minute intervals to local wall-clock minute boundaries.
func NextAlignedIntervalTickTime(now time.Time, interval time.Duration) time.Time {
	if interval <= 0 {
		return now
	}
	if interval%time.Minute != 0 {
		return now.Add(interval)
	}

	loc := now.Location()
	if loc == nil {
		loc = time.Local
	}

	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	currentMinute := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), now.Minute(), 0, 0, loc)
	nextOffset := (currentMinute.Sub(midnight)/interval + 1) * interval
	return midnight.Add(nextOffset)
}
