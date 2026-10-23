package messagingruntime

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

const protocolMetricPrefix = "reliable_messaging_transaction_"

type protocolCollector struct {
	monitor                                                       *protocolMonitor
	gauges                                                        map[string]*prometheus.Desc
	outcomes, checks                                              *prometheus.Desc
	recoveryConfirmed, recoveryAudits, recoveryFailed, reconciled *prometheus.Desc
}

func newProtocolCollector(owner string, monitor *protocolMonitor) *protocolCollector {
	c := &protocolCollector{monitor: monitor, gauges: make(map[string]*prometheus.Desc)}
	labels := prometheus.Labels{"owner": owner}
	descriptions := map[string]string{
		"sampler_running":                             "Host sampler has not actually terminated; includes real DB work after cancellation.",
		"sdk_ready":                                   "SDK readiness independent of host protection and admission.",
		"sdk_stopping":                                "SDK shutdown has started.",
		"sdk_stopped":                                 "SDK client resources actually stopped; host sampler ownership is reported separately.",
		"observation_inflight":                        "A real metadata read is still owned by this sampler; cancellation does not release its capacity.",
		"observation_available":                       "Last metadata sampling succeeded; failed sampling never means zero pending.",
		"observation_fresh":                           "Cached successful metadata is within the host sampling budget.",
		"observation_errors_total":                    "Metadata samples that returned errors or invalid values.",
		"reserved_executions":                         "Prepared executions that have not begun half preparation.",
		"preparing_executions":                        "Prepared executions that have begun half preparation.",
		"prepared_expiry_age_seconds":                 "Oldest time since preparing execution lease expiry; not send age or rollback proof.",
		"confirmation_gap_branches":                   "Terminal branches with half audit but no positive final audit; not Broker backlog.",
		"oldest_confirmation_observation_age_seconds": "Oldest half audit age, otherwise final audit age; exposure age lower bound, not actual Broker pending age.",
		"unaged_confirmation_gap_branches":            "Confirmation audit gaps without an audit timestamp; not fresh zero-age work.",
		"untracked_aborted_branches":                  "Aborted branches with no half outcome; may be unsent or have lost their half audit.",
		"unobserved_committed_branches":               "Committed branches lacking Prepared half audit.",
		"native_limit_active":                         "Host primary has an unresolved native-limit governance notice; not a Broker backlog count.",
		"metadata_conflicts":                          "Contradictory terminal audit metadata or missing witnesses.",
		"admission_protected":                         "Host protection episode is latched until a matching named review.",
		"protection_generation":                       "Process-local protection episode generation; never a metric label.",
		"admission_open":                              "Host new-send admission is open with fresh evidence; Checker may remain live when zero.",
		"active_sends":                                "Actual SDK active sends.", "running_checks": "Actual SDK running check workers.",
		"queued_checks": "Actual SDK waiting checks.", "check_workers_limit": "Configured check worker limit.",
		"check_queue_limit": "Configured waiting check limit.",
	}
	for name, help := range descriptions {
		c.gauges[name] = prometheus.NewDesc(protocolMetricPrefix+name, help, nil, labels)
	}
	c.outcomes = prometheus.NewDesc(protocolMetricPrefix+"outcomes_total", "Cumulative local execution outcomes and half/final branch outcomes; never current backlog.", []string{"stage", "outcome"}, labels)
	c.checks = prometheus.NewDesc(protocolMetricPrefix+"checks_total", "Cumulative SDK check decisions and errors, not pending work.", []string{"outcome"}, labels)
	c.recoveryConfirmed = prometheus.NewDesc(protocolMetricPrefix+"recovery_rpc_confirmed_total", "Positively acknowledged recovery RPCs, not Checker decisions or consumer receipts.", []string{"outcome"}, labels)
	c.recoveryAudits = prometheus.NewDesc(protocolMetricPrefix+"recovery_audits_total", "Positive recovery RPC audit persistence outcomes; missing audit is an observation gap.", []string{"outcome"}, labels)
	c.recoveryFailed = prometheus.NewDesc(protocolMetricPrefix+"recovery_rpc_failures_total", "Go recovery RPC errors, including unsuccessful unspecified decisions.", nil, labels)
	c.reconciled = prometheus.NewDesc(protocolMetricPrefix+"reconciled_executions_total", "Executions durably aborted by SDK maintenance; excludes host business journal maintenance.", nil, labels)
	return c
}
func (c *protocolCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, d := range c.gauges {
		ch <- d
	}
	ch <- c.outcomes
	ch <- c.checks
	ch <- c.recoveryConfirmed
	ch <- c.recoveryAudits
	ch <- c.recoveryFailed
	ch <- c.reconciled
}
func boolean(v bool) float64 {
	if v {
		return 1
	}
	return 0
}
func (c *protocolCollector) Collect(ch chan<- prometheus.Metric) {
	m := c.monitor
	m.mu.Lock()
	p := m.policy
	inflight := m.inflight
	m.mu.Unlock()
	fresh := p.fresh(time.Now(), m.freshness)
	stats := m.stats()
	values := map[string]float64{
		"sampler_running": boolean(m.running()), "sdk_ready": boolean(stats.Ready), "sdk_stopping": boolean(stats.Stopping), "sdk_stopped": boolean(stats.Stopped),
		"observation_inflight":  boolean(inflight),
		"observation_available": boolean(p.available), "observation_fresh": boolean(fresh), "observation_errors_total": float64(p.errors),
		"reserved_executions": float64(p.sample.reserved), "preparing_executions": float64(p.sample.preparing), "prepared_expiry_age_seconds": p.sample.expiryAge,
		"confirmation_gap_branches": float64(p.sample.gaps), "oldest_confirmation_observation_age_seconds": p.sample.confirmationAge,
		"unaged_confirmation_gap_branches": float64(p.sample.unaged), "untracked_aborted_branches": float64(p.sample.untrackedAborted),
		"unobserved_committed_branches": float64(p.sample.unobservedCommitted), "metadata_conflicts": float64(p.sample.conflicts),
		"native_limit_active": boolean(p.sample.nativeBlocked),
		"admission_protected": boolean(p.reason != ""), "protection_generation": float64(p.generation), "admission_open": boolean(m.admitted()),
		"active_sends": float64(stats.ActiveSends), "running_checks": float64(stats.RunningChecks), "queued_checks": float64(stats.QueuedChecks),
		"check_workers_limit": float64(m.workers), "check_queue_limit": float64(m.queue),
	}
	for name, value := range values {
		kind := prometheus.GaugeValue
		if name == "observation_errors_total" {
			kind = prometheus.CounterValue
		}
		ch <- prometheus.MustNewConstMetric(c.gauges[name], kind, value)
	}
	for _, outcome := range []struct {
		stage, name string
		count       int64
	}{
		{"half", "prepared", stats.HalfPreparedResults}, {"half", "rejected", stats.HalfRejectedResults}, {"half", "unknown", stats.HalfUnknownResults},
		{"local", "committed", stats.LocalCommittedResults}, {"local", "aborted", stats.LocalAbortedResults}, {"local", "unknown", stats.LocalUnknownResults},
		{"final", "commit_confirmed", stats.FinalCommitResults}, {"final", "rollback_confirmed", stats.FinalRollbackResults}, {"final", "unknown", stats.FinalUnknownResults},
	} {
		ch <- prometheus.MustNewConstMetric(c.outcomes, prometheus.CounterValue, float64(outcome.count), outcome.stage, outcome.name)
	}
	for _, decision := range []struct {
		name  string
		count int64
	}{
		{"commit", stats.CheckCommit}, {"rollback", stats.CheckRollback}, {"unknown", stats.CheckUnknown}, {"invalid", stats.InvalidChecks},
		{"rejected", stats.CheckRejected}, {"database_error", stats.DatabaseErrors}, {"callback_error", stats.CallbackErrors},
	} {
		ch <- prometheus.MustNewConstMetric(c.checks, prometheus.CounterValue, float64(decision.count), decision.name)
	}
	for _, x := range []struct {
		name  string
		count int64
	}{{"commit", stats.RecoveryCommitConfirmed}, {"rollback", stats.RecoveryRollbackConfirmed}} {
		ch <- prometheus.MustNewConstMetric(c.recoveryConfirmed, prometheus.CounterValue, float64(x.count), x.name)
	}
	for _, x := range []struct {
		name  string
		count int64
	}{{"recorded", stats.RecoveryAuditRecorded}, {"error", stats.RecoveryAuditErrors}, {"unsupported", stats.RecoveryAuditUnsupported}} {
		ch <- prometheus.MustNewConstMetric(c.recoveryAudits, prometheus.CounterValue, float64(x.count), x.name)
	}
	ch <- prometheus.MustNewConstMetric(c.recoveryFailed, prometheus.CounterValue, float64(stats.RecoveryRPCFailed))
	ch <- prometheus.MustNewConstMetric(c.reconciled, prometheus.CounterValue, float64(stats.Reconciled))

}
