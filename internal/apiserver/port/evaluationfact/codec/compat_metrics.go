package codec

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Offline condition (ops / EV-R012):
// when qs_evaluation_outcome_schema_decode_total{schema=~"0|1"} stays flat
// (rate≈0) for a sustained window (e.g. 14d) across environments, remove the
// corresponding schema-0/1 decoder branches in this package.
var outcomeSchemaDecodeTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: "qs",
	Subsystem: "evaluation",
	Name:      "outcome_schema_decode_total",
	Help:      "Durable Evaluation Outcome DecodeExecution hits by schema_version (EV-R012).",
}, []string{"schema"})

func observeOutcomeSchemaDecode(schema uint) {
	outcomeSchemaDecodeTotal.WithLabelValues(fmt.Sprintf("%d", schema)).Inc()
}
