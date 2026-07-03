package result

import (
	"fmt"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/report"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// EventAssembler 事件装配器。
type EventAssembler interface {
	Key() evaluation.EvaluatorKey
	BuildSuccessEvents(outcome Outcome, rpt *domainReport.InterpretReport) []event.DomainEvent
}

type EventAssemblerRegistry interface {
	Resolve(key evaluation.EvaluatorKey) EventAssembler
}

type mutableEventAssemblerRegistry struct {
	items map[evaluation.EvaluatorKey]EventAssembler
}

func NewEventAssemblerRegistry(assemblers ...EventAssembler) (*mutableEventAssemblerRegistry, error) {
	registry := &mutableEventAssemblerRegistry{items: make(map[evaluation.EvaluatorKey]EventAssembler)}
	for _, assembler := range assemblers {
		if err := registry.Register(assembler); err != nil {
			return nil, err
		}
	}
	return registry, nil
}

func (r *mutableEventAssemblerRegistry) Register(assembler EventAssembler) error {
	if assembler == nil {
		return fmt.Errorf("evaluation event assembler is nil")
	}
	key := assembler.Key()
	if key.IsZero() {
		return fmt.Errorf("evaluation event assembler key is empty")
	}
	if _, exists := r.items[key]; exists {
		return fmt.Errorf("evaluation event assembler already registered for key %s", key)
	}
	r.items[key] = assembler
	return nil
}

func (r *mutableEventAssemblerRegistry) Resolve(key evaluation.EvaluatorKey) EventAssembler {
	if r == nil {
		return GenericEventAssembler{}
	}
	if assembler, ok := r.items[key]; ok {
		return assembler
	}
	return GenericEventAssembler{}
}

type GenericEventAssembler struct{}

func (GenericEventAssembler) Key() evaluation.EvaluatorKey {
	return evaluation.EvaluatorKey{}
}

func (GenericEventAssembler) BuildSuccessEvents(outcome Outcome, rpt *domainReport.InterpretReport) []event.DomainEvent {
	if outcome.Assessment == nil || outcome.Execution == nil {
		return nil
	}
	now := time.Now()
	events := []event.DomainEvent{buildInterpretedOutcomeEvent(outcome, rpt, now)}
	if generated := buildReportGeneratedOutcomeEvent(outcome, rpt, now); generated != nil {
		events = append(events, generated)
	}
	if footprint := buildFootprintReportGeneratedEvent(outcome, rpt, now); footprint != nil {
		events = append(events, footprint)
	}
	return events
}

// ScaleEventAssembler is kept for explicit scale registration in tests.
type ScaleEventAssembler struct{}

func (ScaleEventAssembler) Key() evaluation.EvaluatorKey {
	return evaluation.EvaluatorKeyScaleDefault
}

// BuildSuccessEvents 构建 Scale 成功事件，新写路径只发布 outcome 事件。
func (ScaleEventAssembler) BuildSuccessEvents(outcome Outcome, rpt *domainReport.InterpretReport) []event.DomainEvent {
	return (GenericEventAssembler{}).BuildSuccessEvents(outcome, rpt)
}
