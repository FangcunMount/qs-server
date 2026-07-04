package systemgovernance

import (
	"context"
	"sort"
	"time"

	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	outboxport "github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
)

const pendingOldestAgeWarning = 5 * time.Minute

// EventDrainProjection is the diagnostic view derived from event/outbox status.
type EventDrainProjection struct {
	Summary       EventDrainSummary
	OutboxRows    []EventOutboxRow
	EventTypeRows []EventTypeRow
	Signals       []Signal
}

// EventDrainEvaluator turns raw outbox snapshots into operator-facing signals and rows.
type EventDrainEvaluator struct {
	evidence MetricEvidenceReader
}

func NewEventDrainEvaluator(metrics MetricsReader) *EventDrainEvaluator {
	return &EventDrainEvaluator{evidence: NewMetricEvidenceReader(metrics)}
}

func (e *EventDrainEvaluator) Evaluate(
	ctx context.Context,
	snapshot *appEventing.StatusSnapshot,
	eventTypes []EventTypeStatusGroup,
	window string,
	evalAt time.Time,
) EventDrainProjection {
	projection := EventDrainProjection{
		OutboxRows:    []EventOutboxRow{},
		EventTypeRows: []EventTypeRow{},
		Signals:       []Signal{},
	}
	if snapshot != nil {
		projection.Summary.OutboxCount = len(snapshot.Outboxes)
		for _, outbox := range snapshot.Outboxes {
			row := e.projectOutboxRow(ctx, outbox, window, evalAt)
			projection.OutboxRows = append(projection.OutboxRows, row)
			projection.Summary.PendingCount += row.PendingCount
			projection.Summary.FailedCount += row.FailedCount
			if row.Degraded {
				projection.Summary.DegradedReaderCount++
			}
			if row.OldestPendingAgeSeconds > projection.Summary.OldestPendingAgeSeconds {
				projection.Summary.OldestPendingAgeSeconds = row.OldestPendingAgeSeconds
			}
			projection.Signals = append(projection.Signals, outboxSignals(row)...)
		}
	}
	for _, group := range eventTypes {
		rows := e.projectEventTypeRows(ctx, group, window, evalAt)
		for _, row := range rows {
			if row.StatusIsStale() {
				projection.Summary.StaleEventTypeCount++
			}
			if row.Reason != "" && row.EventType == "reader_error" {
				projection.Summary.ReaderErrorCount++
			}
			projection.EventTypeRows = append(projection.EventTypeRows, row)
			projection.Signals = append(projection.Signals, eventTypeSignals(row)...)
		}
	}
	sortEventOutboxRows(projection.OutboxRows)
	sortEventTypeRows(projection.EventTypeRows)
	projection.Signals = SortSignals(projection.Signals)
	return projection
}

func (e *EventDrainEvaluator) projectOutboxRow(
	ctx context.Context,
	outbox appEventing.OutboxSummary,
	window string,
	evalAt time.Time,
) EventOutboxRow {
	row := EventOutboxRow{
		Name:     outbox.Name,
		Store:    nonEmpty(outbox.Store, outbox.Name, "outbox"),
		Degraded: outbox.Degraded,
		Severity: SeverityHealthy,
		Reason:   outbox.Error,
	}
	for _, bucket := range outbox.Buckets {
		switch bucket.Status {
		case "pending":
			row.PendingCount += bucket.Count
			if bucket.OldestAgeSeconds > row.OldestPendingAgeSeconds {
				row.OldestPendingAgeSeconds = bucket.OldestAgeSeconds
			}
		case "failed":
			row.FailedCount += bucket.Count
		case "publishing":
			row.PublishingCount += bucket.Count
		}
	}
	switch {
	case row.Degraded || row.FailedCount > 0:
		row.Severity = SeverityCritical
	case row.PendingCount > 0 && row.OldestPendingAgeSeconds >= 15*time.Minute.Seconds():
		row.Severity = SeverityCritical
	case row.PendingCount > 0 && row.OldestPendingAgeSeconds >= pendingOldestAgeWarning.Seconds():
		row.Severity = SeverityWarning
	}
	row.MetricEvidence = e.outboxMetricEvidence(ctx, row.Store, window, evalAt)
	return row
}

func (e *EventDrainEvaluator) outboxMetricEvidence(ctx context.Context, store, window string, evalAt time.Time) []MetricEvidence {
	if e == nil {
		return nil
	}
	items := make([]MetricEvidence, 0, 3)
	if item, ok := e.evidence.EventOutboxPendingBacklog(ctx, store, window, evalAt); ok {
		items = append(items, item)
	}
	if item, ok := e.evidence.EventOutboxPendingOldestAge(ctx, store, window, evalAt); ok {
		items = append(items, item)
	}
	if item, ok := e.evidence.EventOutboxStatusScrapeFailure(ctx, store, window, evalAt); ok {
		items = append(items, item)
	}
	return items
}

func outboxSignals(row EventOutboxRow) []Signal {
	signals := make([]Signal, 0, 3)
	if row.Degraded {
		signals = append(signals, Signal{
			ID:       "events.outbox.degraded." + row.Name,
			Domain:   DomainEvents,
			Severity: SeverityCritical,
			Status:   "degraded",
			Title:    "Outbox status reader degraded: " + row.Name,
			Evidence: map[string]interface{}{
				"store": row.Store,
				"error": row.Reason,
			},
			MetricEvidence: row.MetricEvidence,
			DashboardKey:   "events_outbox",
		})
	}
	if row.FailedCount > 0 {
		signals = append(signals, Signal{
			ID:       "events.outbox.failed." + row.Name,
			Domain:   DomainEvents,
			Severity: SeverityCritical,
			Status:   "failed",
			Title:    "Outbox has failed events",
			Evidence: map[string]interface{}{
				"store":  row.Store,
				"status": "failed",
				"count":  row.FailedCount,
			},
			DashboardKey: "events_outbox",
		})
	}
	if row.PendingCount > 0 && row.OldestPendingAgeSeconds >= pendingOldestAgeWarning.Seconds() {
		severity := SeverityWarning
		if row.OldestPendingAgeSeconds >= 15*time.Minute.Seconds() {
			severity = SeverityCritical
		}
		signals = append(signals, Signal{
			ID:       "events.outbox.pending_stale." + row.Name,
			Domain:   DomainEvents,
			Severity: severity,
			Status:   "pending_stale",
			Title:    "Pending outbox backlog is aging",
			Evidence: map[string]interface{}{
				"store":                 row.Store,
				"count":                 row.PendingCount,
				"oldest_age_seconds":    row.OldestPendingAgeSeconds,
				"warning_after_seconds": pendingOldestAgeWarning.Seconds(),
			},
			MetricEvidence: row.MetricEvidence,
			DashboardKey:   "events_outbox",
		})
	}
	return signals
}

func (e *EventDrainEvaluator) projectEventTypeRows(
	ctx context.Context,
	group EventTypeStatusGroup,
	window string,
	evalAt time.Time,
) []EventTypeRow {
	if group.Error != "" && len(group.Buckets) == 0 {
		return []EventTypeRow{{
			Store:     group.Store,
			EventType: "reader_error",
			Severity:  SeverityWarning,
			Degraded:  true,
			Reason:    group.Error,
		}}
	}
	rows := map[string]EventTypeRow{}
	for _, bucket := range group.Buckets {
		key := bucket.EventType
		row := rows[key]
		if row.EventType == "" {
			row = EventTypeRow{
				Store:     group.Store,
				EventType: bucket.EventType,
				Severity:  SeverityHealthy,
				Reason:    group.Error,
			}
		}
		switch bucket.Status {
		case "pending":
			row.PendingCount += bucket.Count
			age := eventTypeAgeSeconds(bucket, evalAt)
			if age > row.OldestAgeSeconds {
				row.OldestAgeSeconds = age
			}
		case "failed":
			row.FailedCount += bucket.Count
			if bucket.Count > 0 {
				row.Degraded = true
			}
		}
		if group.Error != "" {
			row.Degraded = true
		}
		row.Severity = eventTypeSeverity(row)
		rows[key] = row
	}
	result := make([]EventTypeRow, 0, len(rows))
	for _, row := range rows {
		row.MetricEvidence = e.eventTypeMetricEvidence(ctx, row.Store, row.EventType, window, evalAt)
		result = append(result, row)
	}
	return result
}

func eventTypeAgeSeconds(bucket outboxport.EventTypeStatusBucket, evalAt time.Time) float64 {
	if bucket.OldestCreatedAt == nil {
		return 0
	}
	age := evalAt.Sub(*bucket.OldestCreatedAt).Seconds()
	if age < 0 {
		return 0
	}
	return age
}

func eventTypeSeverity(row EventTypeRow) Severity {
	switch {
	case row.FailedCount > 0:
		return SeverityCritical
	case row.PendingCount > 0 && row.OldestAgeSeconds >= pendingOldestAgeWarning.Seconds():
		return SeverityWarning
	case row.Degraded:
		return SeverityWarning
	default:
		return SeverityHealthy
	}
}

func (r EventTypeRow) StatusIsStale() bool {
	return r.PendingCount > 0 && r.OldestAgeSeconds >= pendingOldestAgeWarning.Seconds()
}

func (e *EventDrainEvaluator) eventTypeMetricEvidence(ctx context.Context, store, eventType, window string, evalAt time.Time) []MetricEvidence {
	if e == nil || eventType == "" || eventType == "reader_error" {
		return nil
	}
	items := make([]MetricEvidence, 0, 2)
	if item, ok := e.evidence.EventTypePendingBacklog(ctx, store, eventType, window, evalAt); ok {
		items = append(items, item)
	}
	if item, ok := e.evidence.EventTypePendingOldestAge(ctx, store, eventType, window, evalAt); ok {
		items = append(items, item)
	}
	return items
}

func eventTypeSignals(row EventTypeRow) []Signal {
	if row.EventType == "reader_error" && row.Reason != "" {
		return []Signal{{
			ID:       "events.type.reader_error." + row.Store,
			Domain:   DomainEvents,
			Severity: SeverityWarning,
			Status:   "event_type_reader_error",
			Title:    "Event type status unavailable",
			Evidence: map[string]interface{}{
				"store": row.Store,
				"error": row.Reason,
			},
		}}
	}
	if !row.StatusIsStale() {
		return nil
	}
	return []Signal{{
		ID:       "events.type.pending_stale." + row.Store + "." + row.EventType,
		Domain:   DomainEvents,
		Severity: SeverityWarning,
		Status:   "event_type_backlog",
		Title:    "Event type backlog is aging: " + row.EventType,
		Evidence: map[string]interface{}{
			"store":              row.Store,
			"event_type":         row.EventType,
			"count":              row.PendingCount,
			"oldest_age_seconds": row.OldestAgeSeconds,
		},
		MetricEvidence: row.MetricEvidence,
		DashboardKey:   "events_outbox_by_type",
	}}
}

func sortEventOutboxRows(rows []EventOutboxRow) {
	sort.SliceStable(rows, func(i, j int) bool {
		if severityRank(rows[i].Severity) != severityRank(rows[j].Severity) {
			return severityRank(rows[i].Severity) > severityRank(rows[j].Severity)
		}
		if rows[i].OldestPendingAgeSeconds != rows[j].OldestPendingAgeSeconds {
			return rows[i].OldestPendingAgeSeconds > rows[j].OldestPendingAgeSeconds
		}
		return rows[i].Name < rows[j].Name
	})
}

func sortEventTypeRows(rows []EventTypeRow) {
	sort.SliceStable(rows, func(i, j int) bool {
		if severityRank(rows[i].Severity) != severityRank(rows[j].Severity) {
			return severityRank(rows[i].Severity) > severityRank(rows[j].Severity)
		}
		if rows[i].OldestAgeSeconds != rows[j].OldestAgeSeconds {
			return rows[i].OldestAgeSeconds > rows[j].OldestAgeSeconds
		}
		if rows[i].Store != rows[j].Store {
			return rows[i].Store < rows[j].Store
		}
		return rows[i].EventType < rows[j].EventType
	})
}
