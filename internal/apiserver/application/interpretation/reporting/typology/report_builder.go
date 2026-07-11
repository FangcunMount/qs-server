package typology

import (
	"context"
	"fmt"

	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	interpretationreporting "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

type ReportBuilder struct {
	registry ReportAdapterRegistry
	key      evaluation.ExecutionIdentity
}

var (
	_ interpretationreporting.ReportBuilder                    = ReportBuilder{}
	_ interpretationreporting.MechanismKeyedReportBuilder      = ReportBuilder{}
	_ interpretationreporting.MultiMechanismKeyedReportBuilder = ReportBuilder{}
)

func NewReportBuilder(algorithm modelcatalog.Algorithm) (ReportBuilder, error) {
	return ReportBuilder{
		registry: DefaultReportAdapterRegistry(),
		key:      evaluation.PersonalityTypologyIdentity(algorithm),
	}, nil
}

func NewConfiguredReportBuilderWithRegistry(registry ReportAdapterRegistry) (ReportBuilder, error) {
	if registry.Len() == 0 {
		return ReportBuilder{}, fmt.Errorf("typology report adapter registry is required")
	}
	return ReportBuilder{
		registry: registry,
		key:      evaluation.ExecutionIdentityPersonalityTypology,
	}, nil
}

func NewConfiguredReportBuilder() (ReportBuilder, error) {
	return NewConfiguredReportBuilderWithRegistry(DefaultReportAdapterRegistry())
}

func NewReportBuilderWithRegistry(registry ReportAdapterRegistry, algorithm modelcatalog.Algorithm) (ReportBuilder, error) {
	if registry.Len() == 0 {
		return ReportBuilder{}, fmt.Errorf("typology report adapter registry is required")
	}
	return ReportBuilder{
		registry: registry,
		key:      evaluation.PersonalityTypologyIdentity(algorithm),
	}, nil
}

func NewLegacyTypologyAliasReportBuilder(configured ReportBuilder, algorithm modelcatalog.Algorithm) (ReportBuilder, error) {
	if configured.registry.Len() == 0 {
		return ReportBuilder{}, fmt.Errorf("configured typology report builder is required")
	}
	return ReportBuilder{
		registry: configured.registry,
		key:      evaluation.PersonalityTypologyIdentity(algorithm),
	}, nil
}

func (b ReportBuilder) ExecutionIdentity() evaluation.ExecutionIdentity {
	return b.key
}

func (b ReportBuilder) Key() evaluation.ExecutionIdentity {
	return b.ExecutionIdentity()
}

func (ReportBuilder) ReportType() domainReport.ReportType {
	return domainReport.ReportTypeStandard
}

func (ReportBuilder) MechanismKey() interpretationreporting.MechanismReportBuilderKey {
	return typologyMechanismKeys()[0]
}

func (ReportBuilder) MechanismKeys() []interpretationreporting.MechanismReportBuilderKey {
	return typologyMechanismKeys()
}

func typologyMechanismKeys() []interpretationreporting.MechanismReportBuilderKey {
	reportType := domainReport.ReportTypeStandard
	return []interpretationreporting.MechanismReportBuilderKey{
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

func (b ReportBuilder) Build(_ context.Context, outcome evaloutcome.Outcome) (*domainReport.InterpretReport, error) {
	if b.registry.Len() == 0 {
		return nil, fmt.Errorf("personality typology report builder is not configured")
	}
	spec, mapping, decisionKind := resolveReportBuildContext(outcome)
	rpt, err := b.registry.build(spec, mapping, decisionKind, outcome)
	if err != nil {
		return nil, err
	}
	return interpretationreporting.AttachReportOutcomeSummary(outcome, rpt), nil
}
