package persistence

import (
	"errors"
	"time"

	domaingeneration "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/generation"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

func observeParticipantRequestAdmission(err error) {
	result := "accepted"
	switch {
	case errors.Is(err, domaingeneration.ErrOrgDailyBudgetExceeded):
		result = "org_daily_budget_exceeded"
	case errors.Is(err, domaingeneration.ErrUserDailyBudgetExceeded):
		result = "user_daily_budget_exceeded"
	case errors.Is(err, domaingeneration.ErrAssessmentDailyBudgetExceeded):
		result = "assessment_daily_budget_exceeded"
	case errors.Is(err, domaingeneration.ErrAlreadyExists):
		result = "semantic_race_reused"
	case err != nil:
		result = "error"
	}
	participantRequestAdmissionTotal.WithLabelValues(result).Inc()
	if err == nil {
		participantReservedProviderInvocationsTotal.Inc()
	}
}

func observeParticipantExecutionAdmission(err error) {
	result := "accepted"
	switch {
	case errors.Is(err, domaingeneration.ErrOrgActiveCapacityExceeded):
		result = "org_active_capacity_exceeded"
	case errors.Is(err, domaingeneration.ErrUserActiveCapacityExceeded):
		result = "user_active_capacity_exceeded"
	case errors.Is(err, domaingeneration.ErrAssessmentActiveCapacityExceeded):
		result = "assessment_active_capacity_exceeded"
	case errors.Is(err, domaingeneration.ErrConflict):
		result = "race"
	case err != nil:
		result = "error"
	}
	participantExecutionAdmissionTotal.WithLabelValues(result).Inc()
	if err == nil {
		participantActiveSlotsAcquiredTotal.Inc()
	}
}

func observeParticipantActiveSlotRelease(err error) {
	result := "released"
	if err != nil {
		result = "error"
	}
	participantActiveSlotReleaseTotal.WithLabelValues(result).Inc()
}

func observeParticipantExecutionStarted(createdAt, startedAt time.Time) {
	if createdAt.IsZero() || startedAt.Before(createdAt) {
		return
	}
	participantQueueDurationSeconds.Observe(startedAt.Sub(createdAt).Seconds())
}

func observeParticipantTerminal(outcome string, createdAt, finishedAt time.Time) {
	switch outcome {
	case "generated", "failed":
	default:
		outcome = "unknown"
	}
	participantTerminalTotal.WithLabelValues(outcome).Inc()
	if createdAt.IsZero() || finishedAt.Before(createdAt) {
		return
	}
	participantEndToEndDurationSeconds.WithLabelValues(outcome).Observe(finishedAt.Sub(createdAt).Seconds())
}

var (
	participantRequestAdmissionTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "qs", Subsystem: "ai_explanation_participant", Name: "request_admission_total",
		Help: "Participant AI explanation first-request admission decisions by low-cardinality result.",
	}, []string{"result"})
	participantReservedProviderInvocationsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "qs", Subsystem: "ai_explanation_participant", Name: "reserved_provider_invocations_total",
		Help: "Provider invocations durably reserved by accepted first-time participant AI explanation Generations.",
	})
	participantExecutionAdmissionTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "qs", Subsystem: "ai_explanation_participant", Name: "execution_admission_total",
		Help: "Participant AI explanation distributed Provider execution admission decisions by low-cardinality result.",
	}, []string{"result"})
	participantActiveSlotsAcquiredTotal = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "qs", Subsystem: "ai_explanation_participant", Name: "active_slots_acquired_total",
		Help: "Participant AI explanation Provider execution slots durably acquired.",
	})
	participantActiveSlotReleaseTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "qs", Subsystem: "ai_explanation_participant", Name: "active_slot_release_total",
		Help: "Participant AI explanation Provider execution slot release outcomes.",
	}, []string{"result"})
	participantQueueDurationSeconds = promauto.NewHistogram(prometheus.HistogramOpts{
		Namespace: "qs", Subsystem: "ai_explanation_participant", Name: "queue_duration_seconds",
		Help:    "Time from participant AI explanation request acceptance to durable Provider execution start.",
		Buckets: []float64{0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10, 30, 60, 120, 300, 900},
	})
	participantTerminalTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "qs", Subsystem: "ai_explanation_participant", Name: "terminal_total",
		Help: "Participant AI explanation durable terminal transitions by bounded outcome.",
	}, []string{"outcome"})
	participantEndToEndDurationSeconds = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "qs", Subsystem: "ai_explanation_participant", Name: "end_to_end_duration_seconds",
		Help:    "Time from participant AI explanation request acceptance to durable terminal state.",
		Buckets: []float64{0.1, 0.25, 0.5, 1, 2, 5, 10, 20, 40, 80, 160, 300, 600, 1200},
	}, []string{"outcome"})
)
