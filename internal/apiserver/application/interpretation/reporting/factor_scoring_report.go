package reporting

import (
	"context"
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	interpinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/input"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	reportscore "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/scoring"
)

// FactorScoringReportBuilder only consumes Interpretation facts. Conversion
// from EvaluationOutcome and its model snapshots happens before this boundary.
type FactorScoringReportBuilder struct {
	composer domainReport.ReportBuilder
}

func NewFactorScoringReportBuilder(composer domainReport.ReportBuilder) FactorScoringReportBuilder {
	return FactorScoringReportBuilder{composer: composer}
}

func (b FactorScoringReportBuilder) ExecutionIdentity() evaluation.ExecutionIdentity {
	return evaluation.ExecutionIdentityScaleDefault
}

func (b FactorScoringReportBuilder) Key() evaluation.ExecutionIdentity { return b.ExecutionIdentity() }

func (FactorScoringReportBuilder) ReportType() domainReport.ReportType {
	return domainReport.ReportTypeStandard
}

func (b FactorScoringReportBuilder) Build(ctx context.Context, input interpinput.InterpretationInput) (*report.Draft, error) {
	if b.composer == nil {
		return nil, fmt.Errorf("factor_scoring report builder is not configured")
	}
	if input.FactorScoring == nil {
		return nil, fmt.Errorf("factor_scoring interpretation facts are required")
	}
	_ = ctx
	rpt, err := reportscore.BuildFactorScoringReport(b.composer, reportscore.FactorScoringReportInput{
		AssessmentID: report.ID(input.Association.AssessmentID),
		Scale:        input.FactorScoring.Model,
		TotalScore:   primaryValue(input),
		RiskLevel:    riskLevel(input),
		FactorScores: input.FactorScoring.Factors,
	})
	if err != nil {
		return nil, err
	}
	return DraftFromLegacyReport(input, rpt), nil
}

func primaryValue(input interpinput.InterpretationInput) float64 {
	if input.Result.Primary == nil {
		return 0
	}
	return input.Result.Primary.Value
}

func riskLevel(input interpinput.InterpretationInput) report.RiskLevel {
	if input.Result.Level == nil || !domainReport.IsRiskLevelCode(input.Result.Level.Code) {
		return report.RiskLevelNone
	}
	return report.RiskLevel(input.Result.Level.Code)
}
