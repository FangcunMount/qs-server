package typology

import (
	"context"
	"fmt"

	evaluationresult "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/result"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	evaluationtypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/personality/typology"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/report"
	reporttypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/report/personality/typology"
)

var (
	errAssessmentRequired       = fmt.Errorf("assessment is required")
	errEvaluationResultRequired = fmt.Errorf("evaluation result is required")
)

type ReportBuilder struct{}

var _ evaluationresult.ReportBuilder = ReportBuilder{}

func NewReportBuilder() ReportBuilder {
	return ReportBuilder{}
}

func NewMBTIReportBuilder() evaluationresult.ReportBuilder {
	return algorithmReportBuilder{key: evaluation.EvaluatorKeyMBTI}
}

func NewSBTIReportBuilder() evaluationresult.ReportBuilder {
	return algorithmReportBuilder{key: evaluation.EvaluatorKeySBTI}
}

type algorithmReportBuilder struct {
	key evaluation.EvaluatorKey
}

func (b algorithmReportBuilder) Key() evaluation.EvaluatorKey {
	return b.key
}

func (algorithmReportBuilder) ReportType() domainReport.ReportType {
	return domainReport.ReportTypeStandard
}

func (b algorithmReportBuilder) Build(ctx context.Context, outcome evaluationresult.Outcome) (*domainReport.InterpretReport, error) {
	return (ReportBuilder{}).Build(ctx, outcome)
}

func (ReportBuilder) Key() evaluation.EvaluatorKey {
	return evaluation.EvaluatorKeyMBTI
}

func (ReportBuilder) ReportType() domainReport.ReportType {
	return domainReport.ReportTypeStandard
}

func (ReportBuilder) Build(_ context.Context, outcome evaluationresult.Outcome) (*domainReport.InterpretReport, error) {
	algorithm := resolveTypologyAlgorithm(outcome)
	switch algorithm {
	case assessmentmodel.AlgorithmSBTI:
		return buildSBTIReport(outcome)
	default:
		return buildMBTIReport(outcome)
	}
}

func resolveTypologyAlgorithm(outcome evaluationresult.Outcome) assessmentmodel.Algorithm {
	if outcome.Execution != nil && outcome.Execution.ModelRef.Algorithm() != "" {
		return outcome.Execution.ModelRef.Algorithm()
	}
	result := outcome.LegacyResult()
	if result == nil {
		return assessmentmodel.AlgorithmMBTI
	}
	if result.ModelRef.Algorithm() != "" {
		return result.ModelRef.Algorithm()
	}
	switch result.Detail.Payload.(type) {
	case evaluationtypology.SBTIResultDetail, *evaluationtypology.SBTIResultDetail:
		return assessmentmodel.AlgorithmSBTI
	default:
		return assessmentmodel.AlgorithmMBTI
	}
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
	return evaluationresult.AttachReportOutcomeSummary(outcome, rpt), nil
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
	return evaluationresult.AttachReportOutcomeSummary(outcome, rpt), nil
}
