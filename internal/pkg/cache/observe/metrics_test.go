package observe

import (
	"sort"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

func TestComponentFamilyOperationMetricLabelsStayBounded(t *testing.T) {
	ObserveComponentCacheOperation("qs-apiserver", "static_meta", "get", time.Millisecond, true)
	assertObservedMetricLabels(t, "qs_cache_family_operation_duration_seconds", []string{"component", "family", "op"})
	assertObservedMetricLabels(t, "qs_cache_family_operation_errors_total", []string{"component", "family", "op"})
}

func assertObservedMetricLabels(t *testing.T, name string, want []string) {
	t.Helper()
	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, family := range families {
		if family.GetName() != name || len(family.Metric) == 0 {
			continue
		}
		labels := make([]string, 0, len(family.Metric[0].Label))
		for _, label := range family.Metric[0].Label {
			labels = append(labels, label.GetName())
		}
		sort.Strings(labels)
		if len(labels) != len(want) {
			t.Fatalf("%s labels = %v, want %v", name, labels, want)
		}
		for index := range want {
			if labels[index] != want[index] {
				t.Fatalf("%s labels = %v, want %v", name, labels, want)
			}
		}
		return
	}
	t.Fatalf("metric %s was not gathered", name)
}
