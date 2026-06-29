package statistics

import (
	"sync"

	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// FootprintDurableStagingPolicy controls which footprint events may enter durable outbox.
type FootprintDurableStagingPolicy struct {
	disabled map[string]struct{}
}

var (
	footprintStagingMu sync.RWMutex
	footprintStaging   *FootprintDurableStagingPolicy
)

// DefaultDisabledHighFrequencyFootprintEvents lists footprint events moved to scan projection.
func DefaultDisabledHighFrequencyFootprintEvents() []string {
	return []string{
		eventcatalog.FootprintAnswerSheetSubmitted,
		eventcatalog.FootprintReportGenerated,
	}
}

// NewFootprintDurableStagingPolicy builds a policy from disabled event types.
func NewFootprintDurableStagingPolicy(disabledEventTypes []string) *FootprintDurableStagingPolicy {
	if len(disabledEventTypes) == 0 {
		return &FootprintDurableStagingPolicy{}
	}
	disabled := make(map[string]struct{}, len(disabledEventTypes))
	for _, eventType := range disabledEventTypes {
		if eventType == "" {
			continue
		}
		disabled[eventType] = struct{}{}
	}
	return &FootprintDurableStagingPolicy{disabled: disabled}
}

// Allows reports whether a footprint event type may be staged to durable outbox.
func (p *FootprintDurableStagingPolicy) Allows(eventType string) bool {
	if p == nil || len(p.disabled) == 0 {
		return true
	}
	_, disabled := p.disabled[eventType]
	return !disabled
}

// InstallFootprintDurableStagingPolicy sets the process-wide footprint staging policy.
func InstallFootprintDurableStagingPolicy(policy *FootprintDurableStagingPolicy) {
	footprintStagingMu.Lock()
	footprintStaging = policy
	footprintStagingMu.Unlock()
}

// FootprintEventAllowed checks the installed footprint staging policy.
func FootprintEventAllowed(eventType string) bool {
	footprintStagingMu.RLock()
	policy := footprintStaging
	footprintStagingMu.RUnlock()
	return policy.Allows(eventType)
}

// FilterFootprintStagingEvents removes footprint events blocked by the staging policy.
func FilterFootprintStagingEvents(events []event.DomainEvent) []event.DomainEvent {
	footprintStagingMu.RLock()
	policy := footprintStaging
	footprintStagingMu.RUnlock()
	if policy == nil || len(policy.disabled) == 0 || len(events) == 0 {
		return events
	}
	filtered := make([]event.DomainEvent, 0, len(events))
	for _, evt := range events {
		if evt == nil {
			continue
		}
		if policy.Allows(evt.EventType()) {
			filtered = append(filtered, evt)
		}
	}
	return filtered
}
