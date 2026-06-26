package result

import (
	"fmt"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/report"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// EventAssembler 事件装配器。
type EventAssembler interface {
	Kind() assessment.EvaluationModelKind
	BuildSuccessEvents(outcome Outcome, rpt *domainReport.InterpretReport) []event.DomainEvent
}

type EventAssemblerRegistry interface {
	Resolve(kind assessment.EvaluationModelKind) EventAssembler
}

type mutableEventAssemblerRegistry struct {
	items map[assessment.EvaluationModelKind]EventAssembler
}

func NewEventAssemblerRegistry(assemblers ...EventAssembler) (*mutableEventAssemblerRegistry, error) {
	registry := &mutableEventAssemblerRegistry{items: make(map[assessment.EvaluationModelKind]EventAssembler)}
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
	kind := assembler.Kind()
	if kind == "" {
		return fmt.Errorf("evaluation event assembler kind is empty")
	}
	if _, exists := r.items[kind]; exists {
		return fmt.Errorf("evaluation event assembler already registered for kind %s", kind)
	}
	r.items[kind] = assembler
	return nil
}

func (r *mutableEventAssemblerRegistry) Resolve(kind assessment.EvaluationModelKind) EventAssembler {
	if r == nil {
		return GenericEventAssembler{}
	}
	if assembler, ok := r.items[kind]; ok {
		return assembler
	}
	return GenericEventAssembler{}
}

type GenericEventAssembler struct{}

func (GenericEventAssembler) Kind() assessment.EvaluationModelKind {
	return ""
}

func (GenericEventAssembler) BuildSuccessEvents(outcome Outcome, rpt *domainReport.InterpretReport) []event.DomainEvent {
	if outcome.Assessment == nil || outcome.Result == nil {
		return nil
	}
	now := time.Now()
	events := []event.DomainEvent{buildInterpretedV2Event(outcome, rpt, now)}
	if generated := buildReportGeneratedV2Event(outcome, rpt, now); generated != nil {
		events = append(events, generated)
	}
	if footprint := buildFootprintReportGeneratedEvent(outcome, rpt, now); footprint != nil {
		events = append(events, footprint)
	}
	return events
}

type ScaleEventAssembler struct{}

func (ScaleEventAssembler) Kind() assessment.EvaluationModelKind {
	return assessment.EvaluationModelKindScale
}

// BuildSuccessEvents 构建 Scale 成功事件，新写路径只发布 v2 outcome 事件。
func (ScaleEventAssembler) BuildSuccessEvents(outcome Outcome, rpt *domainReport.InterpretReport) []event.DomainEvent {
	if outcome.Assessment == nil || outcome.Result == nil || rpt == nil {
		return nil
	}
	now := time.Now()
	return []event.DomainEvent{
		buildInterpretedV2Event(outcome, rpt, now),
		buildReportGeneratedV2Event(outcome, rpt, now),
		buildFootprintReportGeneratedEvent(outcome, rpt, now),
	}
}
