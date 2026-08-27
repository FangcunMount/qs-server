package participant

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

func observeParticipantRequest(result string) {
	participantRequestTotal.WithLabelValues(result).Inc()
}

var participantRequestTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: "qs", Subsystem: "ai_explanation_participant", Name: "requests_total",
	Help: "Participant AI explanation requests by bounded lifecycle result.",
}, []string{"result"})
