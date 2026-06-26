package typology

import (
	"context"
	"fmt"

	evaluationresult "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/result"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/report"
)

type ReportBuilder struct {
	runner *algorithmRunner
	key    evaluation.EvaluatorKey
}

var _ evaluationresult.ReportBuilder = ReportBuilder{}

func NewReportBuilder(algorithm assessmentmodel.Algorithm) (ReportBuilder, error) {
	return NewReportBuilderWithRegistry(mustDefaultModuleRegistry(), algorithm)
}

func NewConfiguredReportBuilderWithRegistry(registry ModuleRegistry) (ReportBuilder, error) {
	runner, err := registry.runnerForKey(evaluation.EvaluatorKeyPersonalityTypology)
	if err != nil {
		return ReportBuilder{}, err
	}
	return ReportBuilder{
		runner: &runner,
		key:    evaluation.EvaluatorKeyPersonalityTypology,
	}, nil
}

func NewConfiguredReportBuilder() (ReportBuilder, error) {
	return NewConfiguredReportBuilderWithRegistry(mustDefaultModuleRegistry())
}

func NewReportBuilderWithRegistry(registry ModuleRegistry, algorithm assessmentmodel.Algorithm) (ReportBuilder, error) {
	runner, err := algorithmRunnerFor(registry, algorithm)
	if err != nil {
		return ReportBuilder{}, err
	}
	return ReportBuilder{
		runner: &runner,
		key:    evaluation.PersonalityTypologyKey(algorithm),
	}, nil
}

func NewLegacyTypologyAliasReportBuilder(configured ReportBuilder, algorithm assessmentmodel.Algorithm) (ReportBuilder, error) {
	if configured.runner == nil {
		return ReportBuilder{}, fmt.Errorf("configured typology report builder is required")
	}
	return ReportBuilder{
		runner: configured.runner,
		key:    evaluation.PersonalityTypologyKey(algorithm),
	}, nil
}

func (b ReportBuilder) Key() evaluation.EvaluatorKey {
	if b.key.IsZero() && b.runner != nil {
		return evaluation.PersonalityTypologyKey(b.runner.algorithm())
	}
	return b.key
}

func (ReportBuilder) ReportType() domainReport.ReportType {
	return domainReport.ReportTypeStandard
}

func (b ReportBuilder) Build(_ context.Context, outcome evaluationresult.Outcome) (*domainReport.InterpretReport, error) {
	if b.runner == nil {
		return nil, fmt.Errorf("personality typology report builder is not configured")
	}
	rpt, err := b.runner.buildReport(outcome)
	if err != nil {
		return nil, err
	}
	return evaluationresult.AttachReportOutcomeSummary(outcome, rpt), nil
}
