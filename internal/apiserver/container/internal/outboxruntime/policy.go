package outboxruntime

import (
	"slices"

	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/outboxpriority"
)

// Policy is the explicit runtime policy shared by an Outbox staging path,
// immediate dispatcher, and relay scheduler.
type Policy struct {
	ImmediateEventTypes []string
	PriorityTiers       [][]string
}

func DefaultPolicy() Policy {
	return Policy{
		ImmediateEventTypes: []string{eventcatalog.AnswerSheetSubmitted, eventcatalog.EvaluationRequested, eventcatalog.EvaluationOutcomeCommitted},
		PriorityTiers:       outboxpriority.ClaimOrder(nil, nil),
	}
}

func (p Policy) AllowsImmediate(eventType string) bool {
	return slices.Contains(p.ImmediateEventTypes, eventType)
}
