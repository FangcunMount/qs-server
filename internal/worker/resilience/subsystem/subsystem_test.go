package subsystem

import (
	"testing"
	"time"
)

func TestSnapshotDoesNotInventWorkerRateOrBackpressureCapabilities(t *testing.T) {
	s, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := s.Snapshot(time.Now())
	if snapshot.InstanceID == "" || len(snapshot.RateLimits) != 0 || len(snapshot.Backpressure) != 0 {
		t.Fatalf("Snapshot() = %+v", snapshot)
	}
	if len(snapshot.DuplicateSuppression) != 1 || !snapshot.DuplicateSuppression[0].Degraded {
		t.Fatalf("duplicate suppression = %+v", snapshot.DuplicateSuppression)
	}
}
