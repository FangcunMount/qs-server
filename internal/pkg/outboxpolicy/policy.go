// Package outboxpolicy holds the process-wide durable-staging policy for the
// transactional outbox. It is a neutral, cross-cutting concern: it only decides
// whether a given event type may enter the durable outbox, so it must not depend
// on any bounded context. Producers (evaluation, survey, ...) and the statistics
// read side share this single policy instead of importing each other.
package outboxpolicy

import (
	"sync"

	"github.com/FangcunMount/qs-server/pkg/event"
)

// Policy controls which event types may enter the durable outbox.
type Policy struct {
	disabled map[string]struct{}
}

var (
	mu        sync.RWMutex
	installed *Policy
)

// NewPolicy builds a policy from the disabled event types.
func NewPolicy(disabledEventTypes []string) *Policy {
	if len(disabledEventTypes) == 0 {
		return &Policy{}
	}
	disabled := make(map[string]struct{}, len(disabledEventTypes))
	for _, eventType := range disabledEventTypes {
		if eventType == "" {
			continue
		}
		disabled[eventType] = struct{}{}
	}
	return &Policy{disabled: disabled}
}

// Allows reports whether an event type may be staged to the durable outbox.
func (p *Policy) Allows(eventType string) bool {
	if p == nil || len(p.disabled) == 0 {
		return true
	}
	_, disabled := p.disabled[eventType]
	return !disabled
}

// Install sets the process-wide durable-staging policy.
func Install(policy *Policy) {
	mu.Lock()
	installed = policy
	mu.Unlock()
}

// Allowed checks the installed policy for the given event type.
func Allowed(eventType string) bool {
	mu.RLock()
	policy := installed
	mu.RUnlock()
	return policy.Allows(eventType)
}

// Filter removes events blocked by the installed staging policy.
func Filter(events []event.DomainEvent) []event.DomainEvent {
	mu.RLock()
	policy := installed
	mu.RUnlock()
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
