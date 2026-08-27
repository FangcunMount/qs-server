package execution

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestOutputValidationMetricsNormalizeResults(t *testing.T) {
	acceptedBefore := testutil.ToFloat64(outputValidationTotal.WithLabelValues(validationResultAccepted))
	unknownBefore := testutil.ToFloat64(outputValidationTotal.WithLabelValues(validationResultUnknown))

	observeOutputValidation(validationResultAccepted, 5*time.Millisecond)
	observeOutputValidation("untrusted-dynamic-result", time.Millisecond)

	if delta := testutil.ToFloat64(outputValidationTotal.WithLabelValues(validationResultAccepted)) - acceptedBefore; delta != 1 {
		t.Fatalf("accepted validation metric delta = %v", delta)
	}
	if delta := testutil.ToFloat64(outputValidationTotal.WithLabelValues(validationResultUnknown)) - unknownBefore; delta != 1 {
		t.Fatalf("unknown validation metric delta = %v", delta)
	}
}
