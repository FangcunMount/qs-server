package projection

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting/registry"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationcompat"
	evaluation "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationruntime"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// MechanismCanonicalEventAssembler 暂存成功事件 用于 一个机制家族。
type MechanismCanonicalEventAssembler struct {
	mechanism registry.MechanismReportBuilderKey
	legacyKey evaluation.ExecutionIdentity
}

// NewMechanismCanonicalEventAssembler registers 按机制键 事件组装器。
func NewMechanismCanonicalEventAssembler(mechanism registry.MechanismReportBuilderKey, legacyKey evaluation.ExecutionIdentity) MechanismCanonicalEventAssembler {
	return MechanismCanonicalEventAssembler{mechanism: mechanism, legacyKey: legacyKey}
}

func (a MechanismCanonicalEventAssembler) ExecutionIdentity() evaluation.ExecutionIdentity {
	return a.legacyKey
}

func (a MechanismCanonicalEventAssembler) Key() evaluation.ExecutionIdentity {
	return a.ExecutionIdentity()
}

func (a MechanismCanonicalEventAssembler) MechanismKey() registry.MechanismReportBuilderKey {
	return a.mechanism
}

func (a MechanismCanonicalEventAssembler) BuildSuccessEvents(outcome evaloutcome.Outcome, rpt *domainReport.InterpretReport) []event.DomainEvent {
	return (GenericEventAssembler{}).BuildSuccessEvents(outcome, rpt)
}

// TypologyMechanismEventAssembler 暂存类型学 成功事件 用于 全部判定粒度键。
type TypologyMechanismEventAssembler struct{}

func (TypologyMechanismEventAssembler) ExecutionIdentity() evaluation.ExecutionIdentity {
	return evaluation.ExecutionIdentityPersonalityTypology
}

func (TypologyMechanismEventAssembler) Key() evaluation.ExecutionIdentity {
	return evaluation.ExecutionIdentityPersonalityTypology
}

func (TypologyMechanismEventAssembler) MechanismKey() registry.MechanismReportBuilderKey {
	return typologyMechanismEventKeys()[0]
}

func (TypologyMechanismEventAssembler) MechanismKeys() []registry.MechanismReportBuilderKey {
	return typologyMechanismEventKeys()
}

func (TypologyMechanismEventAssembler) BuildSuccessEvents(outcome evaloutcome.Outcome, rpt *domainReport.InterpretReport) []event.DomainEvent {
	return (GenericEventAssembler{}).BuildSuccessEvents(outcome, rpt)
}

func typologyMechanismEventKeys() []registry.MechanismReportBuilderKey {
	reportType := domainReport.ReportTypeStandard
	return []registry.MechanismReportBuilderKey{
		{
			AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorClassification,
			DecisionKind:    modelcatalog.DecisionKindPoleComposition,
			ReportType:      reportType,
		},
		{
			AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorClassification,
			DecisionKind:    modelcatalog.DecisionKindTraitProfile,
			ReportType:      reportType,
		},
		{
			AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorClassification,
			DecisionKind:    modelcatalog.DecisionKindNearestPattern,
			ReportType:      reportType,
		},
	}
}

// DefaultMechanismEventAssemblers 返回规范 按机制键 事件组装器。
func DefaultMechanismEventAssemblers() []EventAssembler {
	return []EventAssembler{
		NewMechanismCanonicalEventAssembler(registry.MechanismReportBuilderKey{
			AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorScoring,
			DecisionKind:    modelcatalog.DecisionKindScoreRange,
			ReportType:      domainReport.ReportTypeStandard,
		}, evaluation.ExecutionIdentityScaleDefault),
		NewMechanismCanonicalEventAssembler(registry.MechanismReportBuilderKey{
			AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorNorm,
			DecisionKind:    modelcatalog.DecisionKindNormLookup,
			ReportType:      domainReport.ReportTypeStandard,
		}, evaluation.ExecutionIdentityBehavioralRatingDefault),
		NewMechanismCanonicalEventAssembler(registry.MechanismReportBuilderKey{
			AlgorithmFamily: modelcatalog.AlgorithmFamilyTaskPerformance,
			DecisionKind:    modelcatalog.DecisionKindAbilityLevel,
			ReportType:      domainReport.ReportTypeStandard,
		}, evaluation.ExecutionIdentityCognitiveDefault),
		TypologyMechanismEventAssembler{},
	}
}
