package reporting

import (
	"fmt"
	"time"

	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// EventAssembler stages interpretation success events for outbox persistence.
type EventAssembler interface {
	Key() evaluation.EvaluatorKey
	BuildSuccessEvents(outcome evaloutcome.Outcome, rpt *domainReport.InterpretReport) []event.DomainEvent
}

// EventAssemblerRegistry resolves event assemblers by evaluator key or mechanism key.
type EventAssemblerRegistry interface {
	Resolve(key evaluation.EvaluatorKey) EventAssembler
	ResolveByMechanism(key MechanismReportBuilderKey) EventAssembler
}

type mutableEventAssemblerRegistry struct {
	items          map[evaluation.EvaluatorKey]EventAssembler
	mechanismItems map[MechanismReportBuilderKey]EventAssembler
}

// NewEventAssemblerRegistry creates a registry from the given assemblers.
func NewEventAssemblerRegistry(assemblers ...EventAssembler) (*mutableEventAssemblerRegistry, error) {
	registry := &mutableEventAssemblerRegistry{
		items:          make(map[evaluation.EvaluatorKey]EventAssembler),
		mechanismItems: make(map[MechanismReportBuilderKey]EventAssembler),
	}
	for _, assembler := range assemblers {
		if err := registry.Register(assembler); err != nil {
			return nil, err
		}
	}
	return registry, nil
}

func (r *mutableEventAssemblerRegistry) Register(assembler EventAssembler) error {
	if assembler == nil {
		return fmt.Errorf("interpretation event assembler is nil")
	}
	key := assembler.Key()
	if !key.IsZero() {
		if _, exists := r.items[key]; exists {
			return fmt.Errorf("interpretation event assembler already registered for key %s", key)
		}
		r.items[key] = assembler
	}
	if keyed, ok := assembler.(MechanismKeyedEventAssembler); ok {
		mechanismKeys := []MechanismReportBuilderKey{keyed.MechanismKey()}
		if multi, ok := assembler.(MultiMechanismKeyedEventAssembler); ok {
			mechanismKeys = multi.MechanismKeys()
		}
		for _, mechanismKey := range mechanismKeys {
			if mechanismKey.ReportType == "" {
				mechanismKey.ReportType = domainReport.ReportTypeStandard
			}
			if _, exists := r.mechanismItems[mechanismKey]; exists {
				return fmt.Errorf("interpretation event assembler already registered for mechanism %s", mechanismKey)
			}
			r.mechanismItems[mechanismKey] = assembler
		}
	}
	return nil
}

func (r *mutableEventAssemblerRegistry) Resolve(key evaluation.EvaluatorKey) EventAssembler {
	if r == nil {
		return GenericEventAssembler{}
	}
	if assembler, ok := r.items[key]; ok {
		return assembler
	}
	if mechanismKey, ok := MechanismReportBuilderKeyFromEvaluatorKey(key, domainReport.ReportTypeStandard); ok {
		return r.ResolveByMechanism(mechanismKey)
	}
	return GenericEventAssembler{}
}

func (r *mutableEventAssemblerRegistry) ResolveByMechanism(key MechanismReportBuilderKey) EventAssembler {
	if r == nil {
		return GenericEventAssembler{}
	}
	if key.ReportType == "" {
		key.ReportType = domainReport.ReportTypeStandard
	}
	if assembler, ok := r.mechanismItems[key]; ok {
		return assembler
	}
	familyKey := MechanismReportBuilderKey{
		AlgorithmFamily: key.AlgorithmFamily,
		ReportType:      key.ReportType,
	}
	if assembler, ok := r.mechanismItems[familyKey]; ok {
		return assembler
	}
	return GenericEventAssembler{}
}

// GenericEventAssembler is the default event assembler for all evaluator keys.
type GenericEventAssembler struct{}

func (GenericEventAssembler) Key() evaluation.EvaluatorKey {
	return evaluation.EvaluatorKey{}
}

func (GenericEventAssembler) BuildSuccessEvents(outcome evaloutcome.Outcome, rpt *domainReport.InterpretReport) []event.DomainEvent {
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

func (ScaleEventAssembler) BuildSuccessEvents(outcome evaloutcome.Outcome, rpt *domainReport.InterpretReport) []event.DomainEvent {
	return (GenericEventAssembler{}).BuildSuccessEvents(outcome, rpt)
}

func (ScaleEventAssembler) MechanismKey() MechanismReportBuilderKey {
	return MechanismReportBuilderKey{
		AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorScoring,
		DecisionKind:    modelcatalog.DecisionKindScoreRange,
		ReportType:      domainReport.ReportTypeStandard,
	}
}
