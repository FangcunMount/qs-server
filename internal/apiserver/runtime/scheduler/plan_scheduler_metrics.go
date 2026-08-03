package scheduler

import (
	"strconv"

	planApp "github.com/FangcunMount/qs-server/internal/apiserver/application/plan"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	planSchedulerOrganizationTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "qs", Subsystem: "plan_scheduler", Name: "organization_total",
		Help: "Plan scheduler organization executions grouped by result.",
	}, []string{"result"})
	planSchedulerCandidateTasks = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "qs", Subsystem: "plan_scheduler", Name: "candidate_tasks",
		Help: "Bounded task candidates seen during the current scheduler tick.",
	}, []string{"org_id", "phase"})
	planSchedulerOldestAgeSeconds = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "qs", Subsystem: "plan_scheduler", Name: "oldest_task_age_seconds",
		Help: "Age of the oldest task observed in a scheduler state.",
	}, []string{"org_id", "state"})
	planSchedulerExpirationTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "qs", Subsystem: "plan_scheduler", Name: "expiration_transition_total",
		Help: "Task expiration transitions grouped by reason and result.",
	}, []string{"org_id", "reason", "result"})
	planSchedulerMissedBacklog = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "qs", Subsystem: "plan_scheduler", Name: "missed_backlog_present",
		Help: "Whether at least one pending task remains outside its opening window.",
	}, []string{"org_id"})
)

func observePlanSchedulerOrganization(result string) {
	planSchedulerOrganizationTotal.WithLabelValues(result).Inc()
}

func observePlanSchedulerStats(orgID int64, stats planApp.TaskScheduleStats) {
	org := strconv.FormatInt(orgID, 10)
	planSchedulerCandidateTasks.WithLabelValues(org, "open_eligible").Set(float64(stats.PendingCount))
	planSchedulerCandidateTasks.WithLabelValues(org, "opened_overdue").Set(float64(stats.OpenedOverdueCount))
	planSchedulerCandidateTasks.WithLabelValues(org, "missed_open_window").Set(float64(stats.MissedCandidateCount))
	planSchedulerOldestAgeSeconds.WithLabelValues(org, "pending").Set(float64(stats.OldestPendingAgeSeconds))
	planSchedulerOldestAgeSeconds.WithLabelValues(org, "opened_overdue").Set(float64(stats.OldestOpenedAgeSeconds))
	planSchedulerExpirationTotal.WithLabelValues(org, "entry_timeout", "success").Add(float64(stats.ExpiredCount))
	planSchedulerExpirationTotal.WithLabelValues(org, "entry_timeout", "failure").Add(float64(stats.ExpireFailedCount))
	planSchedulerExpirationTotal.WithLabelValues(org, "missed_open_window", "success").Add(float64(stats.MissedExpiredCount))
	planSchedulerExpirationTotal.WithLabelValues(org, "missed_open_window", "failure").Add(float64(stats.MissedExpireFailedCount))
	planSchedulerMissedBacklog.WithLabelValues(org).Set(boolFloat(stats.MissedBacklogCount > 0))
}

func boolFloat(value bool) float64 {
	if value {
		return 1
	}
	return 0
}
