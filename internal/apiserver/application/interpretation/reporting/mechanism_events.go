package reporting

import (
	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// MechanismCanonicalEventAssembler stages success events for one mechanism family.
type MechanismCanonicalEventAssembler struct {
	mechanism MechanismReportBuilderKey
	legacyKey evaluation.ExecutionIdentity
}

// NewMechanismCanonicalEventAssembler registers a mechanism-keyed event assembler.
func NewMechanismCanonicalEventAssembler(mechanism MechanismReportBuilderKey, legacyKey evaluation.ExecutionIdentity) MechanismCanonicalEventAssembler {
	return MechanismCanonicalEventAssembler{mechanism: mechanism, legacyKey: legacyKey}
}

func (a MechanismCanonicalEventAssembler) ExecutionIdentity() evaluation.ExecutionIdentity {
	return a.legacyKey
}

func (a MechanismCanonicalEventAssembler) Key() evaluation.ExecutionIdentity {
	return a.ExecutionIdentity()
}

func (a MechanismCanonicalEventAssembler) MechanismKey() MechanismReportBuilderKey {
	return a.mechanism
}

func (a MechanismCanonicalEventAssembler) BuildSuccessEvents(outcome evaloutcome.Outcome, rpt *domainReport.InterpretReport) []event.DomainEvent {
	return (GenericEventAssembler{}).BuildSuccessEvents(outcome, rpt)
}

// TypologyMechanismEventAssembler stages typology success events for all decision-granularity keys.
type TypologyMechanismEventAssembler struct{}

func (TypologyMechanismEventAssembler) ExecutionIdentity() evaluation.ExecutionIdentity {
	return evaluation.ExecutionIdentityPersonalityTypology
}

func (TypologyMechanismEventAssembler) Key() evaluation.ExecutionIdentity {
	return evaluation.ExecutionIdentityPersonalityTypology
}

func (TypologyMechanismEventAssembler) MechanismKey() MechanismReportBuilderKey {
	return typologyMechanismEventKeys()[0]
}

func (TypologyMechanismEventAssembler) MechanismKeys() []MechanismReportBuilderKey {
	return typologyMechanismEventKeys()
}

func (TypologyMechanismEventAssembler) BuildSuccessEvents(outcome evaloutcome.Outcome, rpt *domainReport.InterpretReport) []event.DomainEvent {
	return (GenericEventAssembler{}).BuildSuccessEvents(outcome, rpt)
}

func typologyMechanismEventKeys() []MechanismReportBuilderKey {
	reportType := domainReport.ReportTypeStandard
	return []MechanismReportBuilderKey{
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

// DefaultMechanismEventAssemblers returns canonical mechanism-keyed event assemblers.
func DefaultMechanismEventAssemblers() []EventAssembler {
	return []EventAssembler{
		NewMechanismCanonicalEventAssembler(MechanismReportBuilderKey{
			AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorScoring,
			DecisionKind:    modelcatalog.DecisionKindScoreRange,
			ReportType:      domainReport.ReportTypeStandard,
		}, evaluation.ExecutionIdentityScaleDefault),
		NewMechanismCanonicalEventAssembler(MechanismReportBuilderKey{
			AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorNorm,
			DecisionKind:    modelcatalog.DecisionKindNormLookup,
			ReportType:      domainReport.ReportTypeStandard,
		}, evaluation.ExecutionIdentityBehavioralRatingDefault),
		NewMechanismCanonicalEventAssembler(MechanismReportBuilderKey{
			AlgorithmFamily: modelcatalog.AlgorithmFamilyTaskPerformance,
			DecisionKind:    modelcatalog.DecisionKindAbilityLevel,
			ReportType:      domainReport.ReportTypeStandard,
		}, evaluation.ExecutionIdentityCognitiveDefault),
		TypologyMechanismEventAssembler{},
	}
}
