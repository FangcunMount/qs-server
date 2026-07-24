package automation

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/admission"
)

var admissionFailureTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "interpretation_admission_failure_total",
	Help: "Durable Interpretation admission failures by stable reason.",
}, []string{"reason"})

func observeAdmissionFailure(reason admission.Kind) {
	if !reason.IsValid() {
		reason = admission.KindInternalError
	}
	admissionFailureTotal.WithLabelValues(string(reason)).Inc()
}
