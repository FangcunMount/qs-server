package projection

import (
	"fmt"
	"time"

	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting/registry"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// EventAssembler 暂存解释成功事件 用于 outbox 持久化。
type EventAssembler interface {
	ExecutionIdentity() evaluation.ExecutionIdentity
	// Key 是deprecated; 使用 Execution身份()。
	Key() evaluation.ExecutionIdentity
	BuildSuccessEvents(outcome evaloutcome.Outcome, rpt *domainReport.InterpretReport) []event.DomainEvent
}

// MechanismKeyedEventAssembler 暴露机制 路由 元数据 用于 事件组装器。
type MechanismKeyedEventAssembler interface {
	EventAssembler
	MechanismKey() registry.MechanismReportBuilderKey
}

// MultiMechanismKeyedEventAssembler registers 额外 decision-granularity 机制键。
type MultiMechanismKeyedEventAssembler interface {
	MechanismKeyedEventAssembler
	MechanismKeys() []registry.MechanismReportBuilderKey
}

// EventAssemblerRegistry 解析事件组装器 按 评估器键 或 机制键。
type EventAssemblerRegistry interface {
	Resolve(key evaluation.ExecutionIdentity) EventAssembler
	ResolveByMechanism(key registry.MechanismReportBuilderKey) EventAssembler
}

type mutableEventAssemblerRegistry struct {
	items          map[evaluation.ExecutionIdentity]EventAssembler
	mechanismItems map[registry.MechanismReportBuilderKey]EventAssembler
}

// NewEventAssemblerRegistry 创建注册表 从 given assemblers。
func NewEventAssemblerRegistry(assemblers ...EventAssembler) (*mutableEventAssemblerRegistry, error) {
	registry := &mutableEventAssemblerRegistry{
		items:          make(map[evaluation.ExecutionIdentity]EventAssembler),
		mechanismItems: make(map[registry.MechanismReportBuilderKey]EventAssembler),
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
	id := assembler.ExecutionIdentity()
	if !id.IsZero() {
		if _, exists := r.items[id]; exists {
			return fmt.Errorf("interpretation event assembler already registered for identity %s", id)
		}
		r.items[id] = assembler
	}
	if keyed, ok := assembler.(MechanismKeyedEventAssembler); ok {
		mechanismKeys := []registry.MechanismReportBuilderKey{keyed.MechanismKey()}
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

func (r *mutableEventAssemblerRegistry) Resolve(key evaluation.ExecutionIdentity) EventAssembler {
	if r == nil {
		return GenericEventAssembler{}
	}
	if assembler, ok := r.items[key]; ok {
		return assembler
	}
	if mechanismKey, ok := registry.MechanismReportBuilderKeyFromExecutionIdentity(key, domainReport.ReportTypeStandard); ok {
		return r.ResolveByMechanism(mechanismKey)
	}
	return GenericEventAssembler{}
}

func (r *mutableEventAssemblerRegistry) ResolveByMechanism(key registry.MechanismReportBuilderKey) EventAssembler {
	if r == nil {
		return GenericEventAssembler{}
	}
	if key.ReportType == "" {
		key.ReportType = domainReport.ReportTypeStandard
	}
	for _, candidate := range registry.MechanismKeyFallbackCandidates(key) {
		if assembler, ok := r.mechanismItems[candidate]; ok {
			return assembler
		}
	}
	return GenericEventAssembler{}
}

// GenericEventAssembler 是默认 事件组装器 用于 全部评估器键。
type GenericEventAssembler struct{}

func (GenericEventAssembler) ExecutionIdentity() evaluation.ExecutionIdentity {
	return evaluation.ExecutionIdentity{}
}

func (GenericEventAssembler) Key() evaluation.ExecutionIdentity {
	return evaluation.ExecutionIdentity{}
}

func (GenericEventAssembler) BuildSuccessEvents(outcome evaloutcome.Outcome, rpt *domainReport.InterpretReport) []event.DomainEvent {
	if outcome.Assessment == nil || outcome.Execution == nil {
		return nil
	}
	now := time.Now()
	events := make([]event.DomainEvent, 0, 2)
	if generated := buildReportGeneratedOutcomeEvent(outcome, rpt, now); generated != nil {
		events = append(events, generated)
	}
	if footprint := buildFootprintReportGeneratedEvent(outcome, rpt, now); footprint != nil {
		events = append(events, footprint)
	}
	return events
}

// ScaleEventAssembler 是kept 用于 显式 scale registration in tests.
type ScaleEventAssembler struct{}

func (ScaleEventAssembler) ExecutionIdentity() evaluation.ExecutionIdentity {
	return evaluation.ExecutionIdentityScaleDefault
}

func (ScaleEventAssembler) Key() evaluation.ExecutionIdentity {
	return evaluation.ExecutionIdentityScaleDefault
}

func (ScaleEventAssembler) BuildSuccessEvents(outcome evaloutcome.Outcome, rpt *domainReport.InterpretReport) []event.DomainEvent {
	return (GenericEventAssembler{}).BuildSuccessEvents(outcome, rpt)
}

func (ScaleEventAssembler) MechanismKey() registry.MechanismReportBuilderKey {
	return registry.MechanismReportBuilderKey{
		AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorScoring,
		DecisionKind:    modelcatalog.DecisionKindScoreRange,
		ReportType:      domainReport.ReportTypeStandard,
	}
}
