package iamauth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestVersionGuardMetricsDoNotRenewProofOnIAMFailure(t *testing.T) {
	clock := &guardClock{at: time.Now()}
	failed := false
	guard, err := NewVersionGuard(versionReaderFunc(func(context.Context) (int64, error) {
		if failed {
			return 0, errors.New("IAM unavailable")
		}
		return 7, nil
	}), 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	guard.now = clock.now
	successBefore := testutil.ToFloat64(committedVersionReadTotal.WithLabelValues("success"))
	failureBefore := testutil.ToFloat64(committedVersionReadTotal.WithLabelValues("reader_error"))
	rejectedBefore := testutil.ToFloat64(committedVersionProofRejected.WithLabelValues("read_or_retry_error"))
	if _, err := guard.Verify(context.Background()); err != nil {
		t.Fatal(err)
	}
	proofStart := testutil.ToFloat64(committedVersionProofStart)
	if proofStart < float64(clock.now().Unix()) ||
		testutil.ToFloat64(committedVersionReadTotal.WithLabelValues("success")) != successBefore+1 {
		t.Fatal("successful IAM read did not expose its proof start")
	}
	failed = true
	if err := guard.Refresh(context.Background()); err == nil {
		t.Fatal("IAM failure renewed the proof")
	}
	clock.advance(10 * time.Second)
	if _, err := guard.Verify(context.Background()); err == nil {
		t.Fatal("expired proof was accepted after IAM failure")
	}
	if got := testutil.ToFloat64(committedVersionProofStart); got != proofStart {
		t.Fatalf("failed IAM read changed proof start: %v -> %v", proofStart, got)
	}
	if got := testutil.ToFloat64(committedVersionReadTotal.WithLabelValues("reader_error")); got != failureBefore+2 {
		t.Fatalf("IAM failure count = %v, want %v", got, failureBefore+2)
	}
	if got := testutil.ToFloat64(committedVersionProofRejected.WithLabelValues("read_or_retry_error")); got != rejectedBefore+1 {
		t.Fatalf("fail-closed proof rejection count = %v, want %v", got, rejectedBefore+1)
	}
}
