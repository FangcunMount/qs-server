package operatorretirement

import (
	"math"
	"testing"
	"time"
)

func TestRecoveryRequiresCompletedExitAndExactVersions(t *testing.T) {
	now := time.Now().UTC()
	task, _ := New(7, 1, 8, 9, 1, "exit", "retire", now)
	if _, err := NewRecovery(task, 9, 3, 10, "recover", "HQ test identity", now); err == nil {
		t.Fatal("incomplete exit accepted")
	}
	if err := task.MarkRevoked(10, now); err != nil {
		t.Fatal(err)
	}
	if err := task.Complete(10, false, now); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name    string
		actor   int64
		version uint32
		policy  int64
		reason  string
	}{
		{"stale identity", 9, 2, 10, "restore"}, {"stale policy", 9, 3, 9, "restore"}, {"self restore", 8, 3, 10, "restore"}, {"no reason", 9, 3, 10, " "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewRecovery(task, tc.actor, tc.version, tc.policy, "recover", tc.reason, now); err == nil {
				t.Fatal("unsafe recovery accepted")
			}
		})
	}
	got, err := NewRecovery(task, 9, 3, 11, " recover ", " HQ test identity ", now)
	if err != nil || got.RetirementRequestID != "exit" || got.RequestID != "recover" || got.ExpectedVersion != 3 {
		t.Fatal(got, err)
	}
	task.ExpectedVersion = math.MaxUint32 - 1
	if _, err := NewRecovery(task, 9, 0, 10, "overflow", "restore", now); err == nil {
		t.Fatal("version overflow accepted")
	}
}
