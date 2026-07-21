package leasemetrics

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestObserveExpiredLeasesIncrementsCounter(t *testing.T) {
	t.Parallel()

	before := testutil.ToFloat64(ExpiredLeaseObservedTotal)
	ObserveExpiredLeases(3)
	after := testutil.ToFloat64(ExpiredLeaseObservedTotal)
	if delta := after - before; delta != 3 {
		t.Fatalf("expired lease delta = %v, want 3", delta)
	}
}

func TestObserveRecoveryRecordsDuration(t *testing.T) {
	t.Parallel()

	expiredAt := time.Date(2026, 7, 21, 10, 0, 0, 0, time.UTC)
	reclaimedAt := expiredAt.Add(12 * time.Second)
	beforeRecovery := testutil.ToFloat64(LeaseRecoveryTotal)
	ObserveRecovery(expiredAt, reclaimedAt)
	afterRecovery := testutil.ToFloat64(LeaseRecoveryTotal)
	if delta := afterRecovery - beforeRecovery; delta != 1 {
		t.Fatalf("recovery total delta = %v, want 1", delta)
	}
}
