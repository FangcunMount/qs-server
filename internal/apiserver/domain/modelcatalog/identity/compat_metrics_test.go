package identity

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestObserveWritePolicyIncrementsRetainedRead(t *testing.T) {
	t.Parallel()
	labels := []string{string(KindTypology), string(AlgorithmMBTI), string(AlgorithmWriteRetainedRead)}
	before := testutil.ToFloat64(identityWritePolicyTotal.WithLabelValues(labels...))
	ObserveWritePolicy(KindTypology, AlgorithmMBTI)
	after := testutil.ToFloat64(identityWritePolicyTotal.WithLabelValues(labels...))
	if after-before != 1 {
		t.Fatalf("delta = %v, want 1", after-before)
	}
}

func TestObserveAlgorithmFallbackIncrements(t *testing.T) {
	t.Parallel()
	labels := []string{string(KindBehavioralRating), "_empty_", string(AlgorithmBehavioralRatingDefault), "test.site"}
	before := testutil.ToFloat64(identityAlgorithmFallbackTotal.WithLabelValues(labels...))
	ObserveAlgorithmFallback(KindBehavioralRating, "", AlgorithmBehavioralRatingDefault, "test.site")
	after := testutil.ToFloat64(identityAlgorithmFallbackTotal.WithLabelValues(labels...))
	if after-before != 1 {
		t.Fatalf("delta = %v, want 1", after-before)
	}
}
