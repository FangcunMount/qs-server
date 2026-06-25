package mbti

import (
	"context"
	"fmt"

	evaluationresult "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/result"
	evaluationdomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/report"
)

type ReportBuilder struct{}

var _ evaluationresult.ReportBuilder = ReportBuilder{}

func NewReportBuilder() ReportBuilder {
	return ReportBuilder{}
}

func (ReportBuilder) Kind() assessment.EvaluationModelKind {
	return assessment.EvaluationModelKindMBTI
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
	detail, err := evaluationdomain.MBTIResultDetailFromPayload(outcome.Result.Detail.Payload)
	if err != nil {
		return nil, err
	}
	modelCode := ""
	if !outcome.Result.ModelRef.Code().IsEmpty() {
		modelCode = outcome.Result.ModelRef.Code().String()
	}
	return domainReport.BuildMBTIReport(domainReport.MBTIReportInput{
		AssessmentID: domainReport.ID(outcome.Assessment.ID()),
		ModelCode:    modelCode,
		TotalScore:   outcome.Result.TotalScore,
		RiskLevel:    domainReport.RiskLevel(outcome.Result.RiskLevel),
		Detail:       mbtiReportDetail(detail),
	})
}
