package typology

import (
	"context"
	"fmt"

	evaluationresult "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/result"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/report"
	reporttypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/report/personality/typology"
)

type ReportBuilder struct {
	runner *algorithmRunner
}

var _ evaluationresult.ReportBuilder = ReportBuilder{}

func NewReportBuilder(algorithm assessmentmodel.Algorithm) (ReportBuilder, error) {
	runner, err := algorithmRunnerFor(algorithm)
	if err != nil {
		return ReportBuilder{}, err
	}
	return ReportBuilder{runner: &runner}, nil
}

func NewMBTIReportBuilder() evaluationresult.ReportBuilder {
	builder, err := NewReportBuilder(assessmentmodel.AlgorithmMBTI)
	if err != nil {
		panic(err)
	}
	return builder
}

func NewSBTIReportBuilder() evaluationresult.ReportBuilder {
	builder, err := NewReportBuilder(assessmentmodel.AlgorithmSBTI)
	if err != nil {
		panic(err)
	}
	return builder
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

func buildMBTIReport(outcome evaluationresult.Outcome) (*domainReport.InterpretReport, error) {
	input, err := MBTIReportInputFromOutcome(outcome)
	if err != nil {
		return nil, err
	}
	rpt, err := reporttypology.BuildMBTIReport(input)
	if err != nil {
		return nil, err
	}
	return rpt, nil
}

func buildSBTIReport(outcome evaluationresult.Outcome) (*domainReport.InterpretReport, error) {
	input, err := SBTIReportInputFromOutcome(outcome)
	if err != nil {
		return nil, err
	}
	rpt, err := reporttypology.BuildSBTIReport(input)
	if err != nil {
		return nil, err
	}
	return rpt, nil
}
