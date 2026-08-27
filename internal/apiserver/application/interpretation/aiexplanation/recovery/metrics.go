package recovery

import (
	"errors"

	domaingeneration "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/generation"
	domainrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/run"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

func observeParticipantRetryAuthorization(result *Result, err error) {
	label := "error"
	switch {
	case err == nil && result != nil && result.Created:
		label = "created"
	case err == nil && result != nil:
		label = "reused"
	case errors.Is(err, domaingeneration.ErrOrgDailyBudgetExceeded),
		errors.Is(err, domaingeneration.ErrUserDailyBudgetExceeded),
		errors.Is(err, domaingeneration.ErrAssessmentDailyBudgetExceeded):
		label = "capacity_rejected"
	case errors.Is(err, domainrun.ErrRetryNotAllowed):
		label = "not_allowed"
	case errors.Is(err, domainrun.ErrConflict):
		label = "conflict"
	}
	participantRetryAuthorizationTotal.WithLabelValues(label).Inc()
}

var participantRetryAuthorizationTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: "qs",
	Subsystem: "ai_explanation_participant",
	Name:      "retry_authorizations_total",
	Help:      "Participant AI explanation manual retry authorization outcomes by low-cardinality result.",
}, []string{"result"})
