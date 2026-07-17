package observability

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var operationTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "qs_locklease_operation_total",
	Help: "Total lock lease operations grouped by component, workload, operation and result.",
}, []string{"component", "name", "operation", "result"})

func ObserveOperation(component, name, operation, result string) {
	operationTotal.WithLabelValues(component, name, operation, result).Inc()
}
