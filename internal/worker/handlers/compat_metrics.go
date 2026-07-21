package handlers

import (
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Offline condition (ops / EV-R015): when
// qs_worker_evaluation_payload_gate_total{class="legacy_incomplete"} stays flat
// (rate≈0) for a sustained window (e.g. 14d), retire the incomplete-payload
// compatibility path documentation and treat missing model identity as invalid
// for new publishers.
var evaluationPayloadGateTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: "qs",
	Subsystem: "worker",
	Name:      "evaluation_payload_gate_total",
	Help:      "evaluation.requested payload gate classifications before Execute (EV-R015).",
}, []string{"class"})

func observeEvaluationPayloadGate(class eventpayload.PayloadGateClass) {
	if class == "" {
		class = eventpayload.PayloadGateInvalid
	}
	evaluationPayloadGateTotal.WithLabelValues(string(class)).Inc()
}
