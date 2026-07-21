package execute

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
)

func histogramSampleCount(t *testing.T, family, result string) uint64 {
	t.Helper()
	observer, err := runDurationSeconds.GetMetricWithLabelValues(family, result)
	if err != nil {
		t.Fatal(err)
	}
	metric, ok := observer.(prometheus.Metric)
	if !ok {
		t.Fatalf("observer type %T does not implement prometheus.Metric", observer)
	}
	var out dto.Metric
	if err := metric.Write(&out); err != nil {
		t.Fatal(err)
	}
	return out.GetHistogram().GetSampleCount()
}

func TestObserveEvaluationRunDurationRecordsHistogram(t *testing.T) {
	t.Parallel()

	family := "ev_r010_hist_factor_scoring"
	before := histogramSampleCount(t, family, "success")
	observeEvaluationRunDuration(family, "success", 80*time.Millisecond, defaultEvaluationRunLease)
	after := histogramSampleCount(t, family, "success")
	if after-before != 1 {
		t.Fatalf("histogram sample delta = %d, want 1", after-before)
	}
}

func TestObserveEvaluationRunDurationBudgetBreaches(t *testing.T) {
	t.Parallel()

	family := "ev_r010_budget_task_performance"
	before60 := testutil.ToFloat64(runLeaseBudgetBreachTotal.WithLabelValues(family, leaseBudgetThreshold60s))
	before100 := testutil.ToFloat64(runLeaseBudgetBreachTotal.WithLabelValues(family, leaseBudgetThreshold100s))
	beforeLease := testutil.ToFloat64(runLeaseBudgetBreachTotal.WithLabelValues(family, leaseBudgetThresholdLease))

	observeEvaluationRunDuration(family, "success", 70*time.Second, defaultEvaluationRunLease)
	if delta := testutil.ToFloat64(runLeaseBudgetBreachTotal.WithLabelValues(family, leaseBudgetThreshold60s)) - before60; delta != 1 {
		t.Fatalf("60s breach delta = %v, want 1", delta)
	}
	if delta := testutil.ToFloat64(runLeaseBudgetBreachTotal.WithLabelValues(family, leaseBudgetThreshold100s)) - before100; delta != 0 {
		t.Fatalf("100s breach delta = %v, want 0", delta)
	}

	observeEvaluationRunDuration(family, "failed", 130*time.Second, defaultEvaluationRunLease)
	if delta := testutil.ToFloat64(runLeaseBudgetBreachTotal.WithLabelValues(family, leaseBudgetThreshold100s)) - before100; delta != 1 {
		t.Fatalf("100s breach delta = %v, want 1", delta)
	}
	if delta := testutil.ToFloat64(runLeaseBudgetBreachTotal.WithLabelValues(family, leaseBudgetThresholdLease)) - beforeLease; delta != 1 {
		t.Fatalf("lease breach delta = %v, want 1", delta)
	}
}

func TestObserveEvaluationRunDurationDefaultsEmptyLabels(t *testing.T) {
	t.Parallel()

	before := histogramSampleCount(t, "unknown", "unknown")
	observeEvaluationRunDuration("", "", 10*time.Millisecond, 0)
	after := histogramSampleCount(t, "unknown", "unknown")
	if after-before != 1 {
		t.Fatalf("empty labels sample delta = %d, want 1", after-before)
	}
}
