package options

import (
	"testing"
	"time"
)

func TestRetryDispatchLimitsDefaultsAndExplicitPause(t *testing.T) {
	var empty *RetryGovernanceOptions
	defaults, automatic, err := empty.DispatchLimits()
	if err != nil || !automatic || defaults.Interval != 5*time.Second || defaults.Budget != 5*time.Second || defaults.Batch != 50 {
		t.Fatalf("unexpected defaults: %+v automatic=%v err=%v", defaults, automatic, err)
	}
	paused := false
	o := &RetryGovernanceOptions{AutomaticRetryEnabled: &paused, Dispatch: &RetryDispatchOptions{Interval: time.Second, Budget: 2 * time.Second, Batch: 7}}
	actual, automatic, err := o.DispatchLimits()
	if err != nil || automatic || actual != *o.Dispatch {
		t.Fatalf("explicit pause/limits lost: %+v automatic=%v err=%v", actual, automatic, err)
	}
	actual, _, err = (&RetryGovernanceOptions{Dispatch: &RetryDispatchOptions{Batch: 1}}).DispatchLimits()
	if err != nil || actual.Interval != defaults.Interval || actual.Budget != defaults.Budget || actual.Batch != 1 {
		t.Fatal("partial override lost defaults", actual, err)
	}
}

func TestRetryDispatchLimitsRejectInvalidBounds(t *testing.T) {
	for _, limits := range []RetryDispatchOptions{{Interval: -1}, {Budget: -1}, {Batch: -1}, {Batch: 51}} {
		if _, _, err := (&RetryGovernanceOptions{Dispatch: &limits}).DispatchLimits(); err == nil {
			t.Fatalf("invalid bound accepted: %+v", limits)
		}
	}
}
