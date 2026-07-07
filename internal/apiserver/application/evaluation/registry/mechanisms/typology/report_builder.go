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
	runner *algorithmRunner
	key    evaluation.ExecutionIdentity
}

var (
	_ interpretationreporting.ReportBuilder                    = ReportBuilder{}
	_ interpretationreporting.MechanismKeyedReportBuilder      = ReportBuilder{}
	_ interpretationreporting.MultiMechanismKeyedReportBuilder = ReportBuilder{}
)

func NewReportBuilder(algorithm modelcatalog.Algorithm) (ReportBuilder, error) {
	return NewReportBuilderWithRegistry(mustDefaultModuleRegistry(), algorithm)
}

func NewConfiguredReportBuilderWithRegistry(registry ModuleRegistry) (ReportBuilder, error) {
	runner, err := registry.runnerForIdentity(evaluation.ExecutionIdentityPersonalityTypology)
	if err != nil {
		return ReportBuilder{}, err
	}
	return ReportBuilder{
		runner: &runner,
		key:    evaluation.ExecutionIdentityPersonalityTypology,
	}, nil
}

func NewConfiguredReportBuilder() (ReportBuilder, error) {
	return NewConfiguredReportBuilderWithRegistry(mustDefaultModuleRegistry())
}

func NewReportBuilderWithRegistry(registry ModuleRegistry, algorithm modelcatalog.Algorithm) (ReportBuilder, error) {
	runner, err := algorithmRunnerFor(registry, algorithm)
	if err != nil {
		return ReportBuilder{}, err
	}
	return ReportBuilder{
		runner: &runner,
		key:    evaluation.PersonalityTypologyIdentity(algorithm),
	}, nil
}

func NewLegacyTypologyAliasReportBuilder(configured ReportBuilder, algorithm modelcatalog.Algorithm) (ReportBuilder, error) {
	if configured.runner == nil {
		return ReportBuilder{}, fmt.Errorf("configured typology report builder is required")
	}
	return ReportBuilder{
		runner: configured.runner,
		key:    evaluation.PersonalityTypologyIdentity(algorithm),
	}, nil
}

func (b ReportBuilder) ExecutionIdentity() evaluation.ExecutionIdentity {
	if b.key.IsZero() && b.runner != nil {
		return evaluation.PersonalityTypologyIdentity(b.runner.algorithm())
	}
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
	if b.runner == nil {
		return nil, fmt.Errorf("personality typology report builder is not configured")
	}
	rpt, err := b.runner.buildReport(outcome)
	if err != nil {
		return nil, err
	}
	return interpretationreporting.AttachReportOutcomeSummary(outcome, rpt), nil
}
