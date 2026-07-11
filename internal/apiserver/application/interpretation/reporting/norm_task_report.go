package reporting

import (
	"context"
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	interpinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/input"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
)

type NormProfileReportBuilder struct {
	scoring FactorScoringReportBuilder
}

func NewNormProfileReportBuilder(composer domainReport.ReportBuilder) NormProfileReportBuilder {
	return NormProfileReportBuilder{scoring: NewFactorScoringReportBuilder(composer)}
}

func (NormProfileReportBuilder) ExecutionIdentity() evaluation.ExecutionIdentity {
	return evaluation.ExecutionIdentityBehavioralRatingDefault
}

func (NormProfileReportBuilder) Key() evaluation.ExecutionIdentity {
	return evaluation.ExecutionIdentityBehavioralRatingDefault
}

func (NormProfileReportBuilder) ReportType() domainReport.ReportType {
	return domainReport.ReportTypeStandard
}

func (b NormProfileReportBuilder) Build(ctx context.Context, input interpinput.InterpretationInput) (*report.Draft, error) {
	if b.scoring.composer == nil {
		return nil, fmt.Errorf("norm_profile report builder is not configured")
	}
	return b.scoring.Build(ctx, input)
}

type TaskPerformanceReportBuilder struct {
	scoring FactorScoringReportBuilder
}

func NewTaskPerformanceReportBuilder(composer domainReport.ReportBuilder) TaskPerformanceReportBuilder {
	return TaskPerformanceReportBuilder{scoring: NewFactorScoringReportBuilder(composer)}
}

func (TaskPerformanceReportBuilder) ExecutionIdentity() evaluation.ExecutionIdentity {
	return evaluation.ExecutionIdentityCognitiveDefault
}

func (TaskPerformanceReportBuilder) Key() evaluation.ExecutionIdentity {
	return evaluation.ExecutionIdentityCognitiveDefault
}

func (TaskPerformanceReportBuilder) ReportType() domainReport.ReportType {
	return domainReport.ReportTypeStandard
}

func (b TaskPerformanceReportBuilder) Build(ctx context.Context, input interpinput.InterpretationInput) (*report.Draft, error) {
	if b.scoring.composer == nil {
		return nil, fmt.Errorf("task_performance report builder is not configured")
	}
	return b.scoring.Build(ctx, input)
}
