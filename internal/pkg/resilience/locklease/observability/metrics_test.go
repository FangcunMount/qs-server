package observability

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestObserveOperationAcceptsCanceledResult(t *testing.T) {
	counter := operationTotal.WithLabelValues("test-component", "test-workload", "renew", "canceled")
	before := testutil.ToFloat64(counter)

	ObserveOperation("test-component", "test-workload", "renew", "canceled")

	if got := testutil.ToFloat64(counter); got != before+1 {
		t.Fatalf("canceled operation total = %v, want %v", got, before+1)
	}
}
