package observability

import (
	"github.com/prometheus/client_golang/prometheus"
	"testing"
	"time"
)

func bestEffortSample(t *testing.T, name, event, result string) (float64, bool) {
	t.Helper()
	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, family := range families {
		if family.GetName() != name {
			continue
		}
		for _, metric := range family.GetMetric() {
			labels := map[string]string{}
			for _, label := range metric.GetLabel() {
				labels[label.GetName()] = label.GetValue()
			}
			if labels["event_type"] != event || labels["result"] != result {
				continue
			}
			if metric.Counter != nil {
				return metric.GetCounter().GetValue(), true
			}
			return metric.GetGauge().GetValue(), true
		}
	}
	return 0, false
}

func TestBestEffortFirstFailureRecordsTimeWithoutPriorScrape(t *testing.T) {
	// No preceding scrape or metric observation is required to expose the first
	// failure. Its timestamp remains after later success and can expire by age.
	const metric = "qs_worker_best_effort_side_effect_last_failure_timestamp_seconds"
	before := time.Now().Unix()
	ObserveBestEffortSideEffect("task.expired", BestEffortCallFailed)
	value, present := bestEffortSample(t, metric, "task.expired", "call_failed")
	if !present || value < float64(before) || value > float64(time.Now().Unix()) {
		t.Fatalf("first failure timestamp missing or invalid: %v, present=%v", value, present)
	}
	ObserveBestEffortSideEffect("task.expired", BestEffortCallSucceeded)
	after, present := bestEffortSample(t, metric, "task.expired", "call_failed")
	if !present || after != value {
		t.Fatal("later success erased the failure")
	}
	if _, present = bestEffortSample(t, metric, "task.expired", "call_succeeded"); present {
		t.Fatal("success recorded as failure")
	}
	ObserveBestEffortSideEffect("task.opened", BestEffortCallFailed)
	ObserveBestEffortSideEffect("task.expired", BestEffortSideEffectResult("unbounded-result"))
	if _, present = bestEffortSample(t, metric, "task.opened", "call_failed"); present {
		t.Fatal("unsupported event label emitted")
	}
	if _, present = bestEffortSample(t, metric, "task.expired", "unbounded-result"); present {
		t.Fatal("unsupported outcome label emitted")
	}
}
