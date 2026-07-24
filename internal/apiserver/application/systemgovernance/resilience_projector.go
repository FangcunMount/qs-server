package systemgovernance

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/resilience"
)

// ResilienceProjection 是diagnostic pressure-protection 视图。
type ResilienceProjection struct {
	Summary          ResilienceSummary
	QueueRows        []ResilienceQueueRow
	BackpressureRows []ResilienceBackpressureRow
	CapabilityRows   []ResilienceCapabilityRow
	Signals          []Signal
}

// ResilienceProjector 将运行时 快照转换为operator-facing 行 和 信号。
type ResilienceProjector struct {
	evidence MetricEvidenceReader
}

func NewResilienceProjector(metrics MetricsReader) *ResilienceProjector {
	return &ResilienceProjector{evidence: NewMetricEvidenceReader(metrics)}
}

func (p *ResilienceProjector) Evaluate(
	ctx context.Context,
	components map[string]ComponentResilience,
	window string,
	evalAt time.Time,
) ResilienceProjection {
	projection := ResilienceProjection{
		QueueRows:        []ResilienceQueueRow{},
		BackpressureRows: []ResilienceBackpressureRow{},
		CapabilityRows:   []ResilienceCapabilityRow{},
		Signals:          []Signal{},
	}
	names := make([]string, 0, len(components))
	for name := range components {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		component := components[name]
		projection.Summary.ComponentCount++
		if !component.Available {
			projection.Summary.UnavailableComponentCount++
			projection.Signals = append(projection.Signals, resilienceComponentUnavailableSignal(name, component.Reason))
			continue
		}
		if component.Partial {
			projection.Signals = append(projection.Signals, resilienceComponentPartialSignal(name, component))
		}
		if len(component.Instances) > 0 {
			instanceIDs := make([]string, 0, len(component.Instances))
			for instanceID := range component.Instances {
				instanceIDs = append(instanceIDs, instanceID)
			}
			sort.Strings(instanceIDs)
			componentNotReady := false
			for _, instanceID := range instanceIDs {
				snapshot := component.Instances[instanceID]
				if snapshot != nil {
					projection.Summary.InstanceCount++
					componentNotReady = componentNotReady || !snapshot.Summary.Ready
					p.projectComponent(ctx, &projection, name, instanceID, *snapshot, window, evalAt)
				}
			}
			if componentNotReady {
				projection.Summary.NotReadyComponentCount++
			}
			continue
		}
		if component.Snapshot == nil {
			continue
		}
		projection.Summary.InstanceCount++
		if !component.Snapshot.Summary.Ready {
			projection.Summary.NotReadyComponentCount++
		}
		p.projectComponent(ctx, &projection, name, component.Snapshot.InstanceID, *component.Snapshot, window, evalAt)
	}
	sortResilienceQueueRows(projection.QueueRows)
	sortResilienceBackpressureRows(projection.BackpressureRows)
	sortResilienceCapabilityRows(projection.CapabilityRows)
	projection.Signals = SortSignals(projection.Signals)
	return projection
}

func (p *ResilienceProjector) projectComponent(
	ctx context.Context,
	projection *ResilienceProjection,
	component string,
	instanceID string,
	snapshot resilience.RuntimeSnapshot,
	window string,
	evalAt time.Time,
) {
	if !snapshot.Summary.Ready {
		projection.Summary.NotReadyInstanceCount++
		projection.Signals = append(projection.Signals, resilienceRuntimeNotReadySignal(component, instanceID, snapshot))
	}
	for _, queue := range snapshot.Queues {
		row := p.projectQueueRow(ctx, component, instanceID, queue, window, evalAt)
		projection.QueueRows = append(projection.QueueRows, row)
		projection.Summary.QueueCount++
		if row.Utilization > projection.Summary.MaxQueueUtilization {
			projection.Summary.MaxQueueUtilization = row.Utilization
		}
		switch row.Severity {
		case SeverityCritical:
			projection.Summary.CriticalQueueCount++
		case SeverityWarning:
			projection.Summary.WarningQueueCount++
		}
		projection.Signals = append(projection.Signals, resilienceQueueSignals(row)...)
	}
	for _, backpressure := range snapshot.Backpressure {
		row := p.projectBackpressureRow(ctx, component, instanceID, backpressure, window, evalAt)
		projection.BackpressureRows = append(projection.BackpressureRows, row)
		projection.Summary.BackpressureCount++
		if row.Utilization > projection.Summary.MaxBackpressureUtilization {
			projection.Summary.MaxBackpressureUtilization = row.Utilization
		}
		switch row.Severity {
		case SeverityCritical:
			projection.Summary.CriticalBackpressureCount++
		case SeverityWarning:
			projection.Summary.WarningBackpressureCount++
		}
		projection.Signals = append(projection.Signals, resilienceBackpressureSignals(row)...)
	}
	for _, row := range resilienceCapabilityRows(component, instanceID, snapshot) {
		if row.Degraded {
			projection.Summary.DegradedCapabilityCount++
		}
		projection.CapabilityRows = append(projection.CapabilityRows, row)
	}
}

func (p *ResilienceProjector) projectQueueRow(
	ctx context.Context,
	component string,
	instanceID string,
	queue resilience.QueueSnapshot,
	window string,
	evalAt time.Time,
) ResilienceQueueRow {
	utilization := queueUtilization(queue)
	row := ResilienceQueueRow{
		Component:         component,
		InstanceID:        instanceID,
		Name:              queue.Name,
		Strategy:          queue.Strategy,
		Depth:             queue.Depth,
		Capacity:          queue.Capacity,
		Utilization:       utilization,
		StatusCounts:      queue.StatusCounts,
		LifecycleBoundary: queue.LifecycleBoundary,
		Severity:          SeverityHealthy,
	}
	if metric, ok := p.evidence.ResilienceQueueFull(ctx, component, queue, window, evalAt); ok {
		row.MetricEvidence = []MetricEvidence{metric}
	}
	switch {
	case utilization >= 0.9:
		row.Severity = SeverityCritical
		row.Reason = "queue utilization critical"
	case utilization >= 0.7:
		row.Severity = SeverityWarning
		row.Reason = "queue utilization elevated"
	}
	return row
}

func resilienceQueueSignals(row ResilienceQueueRow) []Signal {
	switch row.Severity {
	case SeverityCritical:
		return []Signal{{
			ID:       resilienceRowSignalID("resilience.queue.critical", row.Component, row.InstanceID, row.Name),
			Domain:   DomainResilience,
			Severity: SeverityCritical,
			Status:   "queue_utilization_critical",
			Title:    "Queue utilization critical: " + row.Name,
			Evidence: map[string]interface{}{
				"component":   row.Component,
				"instance_id": row.InstanceID,
				"queue":       row.Name,
				"depth":       row.Depth,
				"capacity":    row.Capacity,
				"utilization": row.Utilization,
			},
			MetricEvidence: row.MetricEvidence,
			DashboardKey:   "resilience_queue",
		}}
	case SeverityWarning:
		return []Signal{{
			ID:       resilienceRowSignalID("resilience.queue.warning", row.Component, row.InstanceID, row.Name),
			Domain:   DomainResilience,
			Severity: SeverityWarning,
			Status:   "queue_utilization_warning",
			Title:    "Queue utilization elevated: " + row.Name,
			Evidence: map[string]interface{}{
				"component":   row.Component,
				"instance_id": row.InstanceID,
				"queue":       row.Name,
				"depth":       row.Depth,
				"capacity":    row.Capacity,
				"utilization": row.Utilization,
			},
			MetricEvidence: row.MetricEvidence,
			DashboardKey:   "resilience_queue",
		}}
	default:
		return nil
	}
}

func (p *ResilienceProjector) projectBackpressureRow(
	ctx context.Context,
	component string,
	instanceID string,
	backpressure resilience.BackpressureSnapshot,
	window string,
	evalAt time.Time,
) ResilienceBackpressureRow {
	utilization := backpressureUtilization(backpressure)
	row := ResilienceBackpressureRow{
		Component:     component,
		InstanceID:    instanceID,
		Name:          backpressure.Name,
		Dependency:    backpressure.Dependency,
		Strategy:      backpressure.Strategy,
		Enabled:       backpressure.Enabled,
		InFlight:      backpressure.InFlight,
		MaxInflight:   backpressure.MaxInflight,
		Utilization:   utilization,
		TimeoutMillis: backpressure.TimeoutMillis,
		Degraded:      backpressure.Degraded,
		Severity:      SeverityHealthy,
		Reason:        backpressure.Reason,
	}
	if metric, ok := p.evidence.ResilienceBackpressureTimeout(ctx, component, backpressure, window, evalAt); ok {
		row.MetricEvidence = []MetricEvidence{metric}
	}
	switch {
	case utilization >= 0.9:
		row.Severity = SeverityCritical
		if row.Reason == "" {
			row.Reason = "backpressure utilization critical"
		}
	case utilization >= 0.8:
		row.Severity = SeverityWarning
		if row.Reason == "" {
			row.Reason = "backpressure utilization elevated"
		}
	case row.Degraded:
		row.Severity = SeverityWarning
	}
	return row
}

func resilienceBackpressureSignals(row ResilienceBackpressureRow) []Signal {
	if row.Utilization < 0.8 {
		return nil
	}
	return []Signal{{
		ID:       resilienceRowSignalID("resilience.backpressure", row.Component, row.InstanceID, row.Name),
		Domain:   DomainResilience,
		Severity: row.Severity,
		Status:   "backpressure_utilization",
		Title:    "Backpressure utilization elevated: " + row.Name,
		Evidence: map[string]interface{}{
			"component":    row.Component,
			"instance_id":  row.InstanceID,
			"name":         row.Name,
			"in_flight":    row.InFlight,
			"max_inflight": row.MaxInflight,
			"utilization":  row.Utilization,
		},
		MetricEvidence: row.MetricEvidence,
		DashboardKey:   "resilience_backpressure",
	}}
}

func resilienceCapabilityRows(component, instanceID string, snapshot resilience.RuntimeSnapshot) []ResilienceCapabilityRow {
	rows := []ResilienceCapabilityRow{}
	appendRows := func(kind resilience.ProtectionKind, items []resilience.CapabilitySnapshot) {
		for _, item := range items {
			rowKind := nonEmpty(item.Kind, kind.String())
			severity := SeverityHealthy
			if item.Degraded {
				severity = SeverityWarning
			}
			rows = append(rows, ResilienceCapabilityRow{
				Component:  component,
				InstanceID: instanceID,
				Kind:       rowKind,
				Name:       item.Name,
				Strategy:   item.Strategy,
				Configured: item.Configured,
				Degraded:   item.Degraded,
				Severity:   severity,
				Reason:     item.Reason,
			})
		}
	}
	appendRows(resilience.ProtectionRateLimit, snapshot.RateLimits)
	appendRows(resilience.ProtectionLock, snapshot.Locks)
	appendRows(resilience.ProtectionIdempotency, snapshot.Idempotency)
	appendRows(resilience.ProtectionDuplicateSuppression, snapshot.DuplicateSuppression)
	return rows
}

func resilienceRuntimeNotReadySignal(component, instanceID string, snapshot resilience.RuntimeSnapshot) Signal {
	return Signal{
		ID:       resilienceRowSignalID("resilience.runtime.not_ready", component, instanceID, ""),
		Domain:   DomainResilience,
		Severity: SeverityWarning,
		Status:   "not_ready",
		Title:    "Resilience runtime not ready: " + component,
		Evidence: map[string]interface{}{
			"component":      component,
			"instance_id":    instanceID,
			"degraded_count": snapshot.Summary.DegradedCount,
		},
		DashboardKey: "resilience_runtime",
	}
}

func resilienceComponentPartialSignal(component string, result ComponentResilience) Signal {
	return Signal{
		ID:       "resilience.component.partial." + component,
		Domain:   DomainResilience,
		Severity: SeverityWarning,
		Status:   "component_partial",
		Title:    "Component resilience snapshot is partial: " + component,
		Evidence: map[string]interface{}{
			"component":                 component,
			"discovered_instance_count": result.DiscoveredInstanceCount,
			"available_instance_count":  result.AvailableInstanceCount,
			"target_errors":             result.TargetErrors,
		},
		DashboardKey: "resilience_runtime",
	}
}

func resilienceRowSignalID(prefix, component, instanceID, name string) string {
	parts := []string{prefix, component}
	if instanceID != "" {
		parts = append(parts, instanceID)
	}
	if name != "" {
		parts = append(parts, name)
	}
	return strings.Join(parts, ".")
}

func resilienceComponentUnavailableSignal(component, reason string) Signal {
	return Signal{
		ID:       "resilience.component.unavailable." + component,
		Domain:   DomainResilience,
		Severity: SeverityWarning,
		Status:   "component_unavailable",
		Title:    "Component resilience snapshot unavailable: " + component,
		Evidence: map[string]interface{}{
			"component": component,
			"reason":    reason,
		},
	}
}

func queueUtilization(queue resilience.QueueSnapshot) float64 {
	if queue.Capacity <= 0 {
		return 0
	}
	return float64(queue.Depth) / float64(queue.Capacity)
}

func backpressureUtilization(bp resilience.BackpressureSnapshot) float64 {
	if bp.MaxInflight <= 0 {
		return 0
	}
	return float64(bp.InFlight) / float64(bp.MaxInflight)
}

func queueDecisionLabels(component string, queue resilience.QueueSnapshot, outcome resilience.Outcome) map[string]string {
	return map[string]string{
		"component": nonEmpty(queue.Component, component, "unknown"),
		"kind":      resilience.ProtectionQueue.String(),
		"scope":     nonEmpty(queue.Name, "default"),
		"resource":  queueResource(queue),
		"strategy":  nonEmpty(queue.Strategy, "default"),
		"outcome":   outcome.String(),
	}
}

func queueResource(queue resilience.QueueSnapshot) string {
	return "default"
}

func backpressureDecisionLabels(component string, bp resilience.BackpressureSnapshot, outcome resilience.Outcome) map[string]string {
	return map[string]string{
		"component": nonEmpty(bp.Component, component, "unknown"),
		"kind":      resilience.ProtectionBackpressure.String(),
		"scope":     nonEmpty(bp.Dependency, bp.Name, "default"),
		"resource":  "downstream",
		"strategy":  nonEmpty(bp.Strategy, "default"),
		"outcome":   outcome.String(),
	}
}

func sortResilienceQueueRows(rows []ResilienceQueueRow) {
	sort.SliceStable(rows, func(i, j int) bool {
		if severityRank(rows[i].Severity) != severityRank(rows[j].Severity) {
			return severityRank(rows[i].Severity) > severityRank(rows[j].Severity)
		}
		if rows[i].Utilization != rows[j].Utilization {
			return rows[i].Utilization > rows[j].Utilization
		}
		if rows[i].Component != rows[j].Component {
			return rows[i].Component < rows[j].Component
		}
		if rows[i].InstanceID != rows[j].InstanceID {
			return rows[i].InstanceID < rows[j].InstanceID
		}
		return rows[i].Name < rows[j].Name
	})
}

func sortResilienceBackpressureRows(rows []ResilienceBackpressureRow) {
	sort.SliceStable(rows, func(i, j int) bool {
		if severityRank(rows[i].Severity) != severityRank(rows[j].Severity) {
			return severityRank(rows[i].Severity) > severityRank(rows[j].Severity)
		}
		if rows[i].Utilization != rows[j].Utilization {
			return rows[i].Utilization > rows[j].Utilization
		}
		if rows[i].Component != rows[j].Component {
			return rows[i].Component < rows[j].Component
		}
		if rows[i].InstanceID != rows[j].InstanceID {
			return rows[i].InstanceID < rows[j].InstanceID
		}
		return rows[i].Name < rows[j].Name
	})
}

func sortResilienceCapabilityRows(rows []ResilienceCapabilityRow) {
	sort.SliceStable(rows, func(i, j int) bool {
		if severityRank(rows[i].Severity) != severityRank(rows[j].Severity) {
			return severityRank(rows[i].Severity) > severityRank(rows[j].Severity)
		}
		if rows[i].Component != rows[j].Component {
			return rows[i].Component < rows[j].Component
		}
		if rows[i].InstanceID != rows[j].InstanceID {
			return rows[i].InstanceID < rows[j].InstanceID
		}
		if rows[i].Kind != rows[j].Kind {
			return rows[i].Kind < rows[j].Kind
		}
		return rows[i].Name < rows[j].Name
	})
}
