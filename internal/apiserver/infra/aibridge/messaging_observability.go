package aibridge

import (
	"context"
	"database/sql"
	"time"

	store "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/aibridge"
	"github.com/prometheus/client_golang/prometheus"
)

// Explicit runtime registration, pull-based collection, no observer task or pool.
type messagingCollector struct {
	db           *sql.DB
	descriptions map[string]*prometheus.Desc
}

func newMessagingCollector(db *sql.DB) *messagingCollector {
	descriptions := map[string]string{
		"observation_available":                         "Whether the complete committed MQ state snapshot is available.",
		"staged_messages":                               "Staged messages including future retry availability.",
		"due_messages":                                  "Due staged or awaiting-receipt rows; not ordering eligibility.",
		"awaiting_receipt_messages":                     "Messages still awaiting business confirmation.",
		"held_messages":                                 "Technically held Outbox messages, not business refusal.",
		"held_inbox_events":                             "Technically held Inbox events, separate from Outbox hold.",
		"oldest_staged_seconds":                         "Oldest staged creation age, future dates clamped to zero.",
		"oldest_awaiting_receipt_seconds":               "Oldest message awaiting business confirmation.",
		"quarantine_authentication_failed_records":      "Retained authentication-failure records, not lifetime errors.",
		"quarantine_identity_conflict_records":          "Retained identity-conflict records.",
		"quarantine_technical_budget_exhausted_records": "Retained technical budget quarantine records.",
		"quarantine_failed_delivery_records":            "Retained failed-delivery quarantine records.",
		"duplicate_observations_available":              "Whether durable duplicate observations exist.",
		"payload_error_observations_available":          "Whether durable payload error observations exist.",
	}
	c := &messagingCollector{db: db, descriptions: map[string]*prometheus.Desc{}}
	for name, help := range descriptions {
		c.descriptions[name] = prometheus.NewDesc("qs_server_ai_mq_"+name, help, nil, nil)
	}
	return c
}
func (c *messagingCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, desc := range c.descriptions {
		ch <- desc
	}
}
func (c *messagingCollector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	values, err := store.MessagingSnapshot(ctx, c.db)
	if err != nil {
		ch <- prometheus.MustNewConstMetric(c.descriptions["observation_available"], prometheus.GaugeValue, 0)
		return
	}
	values["observation_available"] = 1
	for name, value := range values {
		if desc := c.descriptions[name]; desc != nil {
			ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, value)
		}
	}
}
