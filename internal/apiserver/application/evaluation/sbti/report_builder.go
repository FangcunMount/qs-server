package sbti

import (
	"context"
	"fmt"

	evaluationresult "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/result"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	evaluationsbti "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/sbti"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/report"
	reportsbti "github.com/FangcunMount/qs-server/internal/apiserver/domain/report/sbti"
)

type ReportBuilder struct{}

var _ evaluationresult.ReportBuilder = ReportBuilder{}

func NewReportBuilder() ReportBuilder {
	return ReportBuilder{}
}

func (ReportBuilder) Kind() assessment.EvaluationModelKind {
	return assessment.EvaluationModelKindSBTI
}

func (ReportBuilder) ReportType() domainReport.ReportType {
	return domainReport.ReportTypeStandard
}

func (ReportBuilder) Build(_ context.Context, outcome evaluationresult.Outcome) (*domainReport.InterpretReport, error) {
	if outcome.Assessment == nil {
		return nil, fmt.Errorf("assessment is required")
	}
	if outcome.Result == nil {
		return nil, fmt.Errorf("evaluation result is required")
	}
	detail, err := evaluationsbti.ResultDetailFromPayload(outcome.Result.Detail.Payload)
	if err != nil {
		return nil, err
	}
	modelCode := ""
	if !outcome.Result.ModelRef.Code().IsEmpty() {
		modelCode = outcome.Result.ModelRef.Code().String()
	}
	return reportsbti.BuildReport(reportsbti.ReportInput{
		AssessmentID: domainReport.ID(outcome.Assessment.ID()),
		ModelCode:    modelCode,
		TotalScore:   outcome.Result.TotalScore,
		RiskLevel:    domainReport.RiskLevel(outcome.Result.RiskLevel),
		Detail:       sbtiReportDetail(detail),
	})
}
