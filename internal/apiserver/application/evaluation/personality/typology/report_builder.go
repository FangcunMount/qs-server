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
}

var _ evaluationresult.ReportBuilder = ReportBuilder{}

func NewReportBuilder(algorithm assessmentmodel.Algorithm) (ReportBuilder, error) {
	return NewReportBuilderWithRegistry(mustDefaultModuleRegistry(), algorithm)
}

func NewReportBuilderWithRegistry(registry ModuleRegistry, algorithm assessmentmodel.Algorithm) (ReportBuilder, error) {
	runner, err := algorithmRunnerFor(registry, algorithm)
	if err != nil {
		return ReportBuilder{}, err
	}
	return ReportBuilder{runner: &runner}, nil
}

func (b ReportBuilder) Key() evaluation.EvaluatorKey {
	if b.runner == nil {
		return evaluation.EvaluatorKey{}
	}
	return evaluation.PersonalityTypologyKey(b.runner.algorithm())
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
