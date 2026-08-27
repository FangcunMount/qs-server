package evaluation

import (
	"errors"

	domainevaluation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/evaluation"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

func observePromptEvaluationStartAdmission(err error) {
	result := "accepted"
	switch {
	case errors.Is(err, domainevaluation.ErrDailyBudgetExceeded):
		result = "daily_budget_exceeded"
	case errors.Is(err, domainevaluation.ErrOrgConcurrencyExceeded):
		result = "org_concurrency_exceeded"
	case err != nil:
		result = "error"
	}
	promptEvaluationStartAdmissionTotal.WithLabelValues(result).Inc()
	if err == nil {
		promptEvaluationReservedProviderInvocationsTotal.Add(float64(MaxProviderInvocationsV1))
	}
}

var (
	promptEvaluationStartAdmissionTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "qs", Subsystem: "ai_explanation_prompt_evaluation", Name: "start_admission_total",
		Help: "Prompt evaluation start admission decisions by low-cardinality result.",
	}, []string{"result"})
	promptEvaluationReservedProviderInvocationsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "qs", Subsystem: "ai_explanation_prompt_evaluation", Name: "reserved_provider_invocations_total",
		Help: "Worst-case Provider invocations durably reserved by accepted Prompt evaluation starts.",
	})
)
