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
	if outcome.Assessment == nil {
		return nil, fmt.Errorf("assessment is required")
	}
	result := outcome.LegacyResult()
	if result == nil {
		return nil, fmt.Errorf("evaluation result is required")
	}
	algorithm := result.ModelRef.Algorithm()
	if algorithm == "" {
		switch result.Detail.Payload.(type) {
		case evaluationtypology.SBTIResultDetail, *evaluationtypology.SBTIResultDetail:
			algorithm = assessmentmodel.AlgorithmSBTI
		default:
			algorithm = assessmentmodel.AlgorithmMBTI
		}
	}
	switch algorithm {
	case assessmentmodel.AlgorithmSBTI:
		return buildSBTIReport(outcome)
	default:
		return buildMBTIReport(outcome)
	}
}

func buildMBTIReport(outcome evaluationresult.Outcome) (*domainReport.InterpretReport, error) {
	result := outcome.LegacyResult()
	if result == nil {
		return nil, fmt.Errorf("evaluation result is required")
	}
	detail, err := evaluationtypology.MBTIResultDetailFromPayload(result.Detail.Payload)
	if err != nil {
		return nil, err
	}
	modelCode := ""
	if !result.ModelRef.Code().IsEmpty() {
		modelCode = result.ModelRef.Code().String()
	}
	rpt, err := reporttypology.BuildMBTIReport(reporttypology.MBTIReportInput{
		AssessmentID: domainReport.ID(outcome.Assessment.ID()),
		ModelCode:    modelCode,
		TotalScore:   result.TotalScore,
		RiskLevel:    domainReport.RiskLevel(result.RiskLevel),
		Detail:       mbtiReportDetail(detail),
	})
	if err != nil {
		return nil, err
	}
	return evaluationresult.AttachReportOutcomeSummary(outcome, rpt), nil
}

func buildSBTIReport(outcome evaluationresult.Outcome) (*domainReport.InterpretReport, error) {
	result := outcome.LegacyResult()
	if result == nil {
		return nil, fmt.Errorf("evaluation result is required")
	}
	detail, err := evaluationtypology.SBTIResultDetailFromPayload(result.Detail.Payload)
	if err != nil {
		return nil, err
	}
	modelCode := ""
	if !result.ModelRef.Code().IsEmpty() {
		modelCode = result.ModelRef.Code().String()
	}
	rpt, err := reporttypology.BuildSBTIReport(reporttypology.SBTIReportInput{
		AssessmentID: domainReport.ID(outcome.Assessment.ID()),
		ModelCode:    modelCode,
		TotalScore:   result.TotalScore,
		RiskLevel:    domainReport.RiskLevel(result.RiskLevel),
		Detail:       sbtiReportDetail(detail),
	})
	if err != nil {
		return nil, err
	}
	return evaluationresult.AttachReportOutcomeSummary(outcome, rpt), nil
}

func mbtiReportDetail(detail evaluationtypology.MBTIResultDetail) reporttypology.MBTIReportDetail {
	dimensions := make([]reporttypology.MBTIDimensionReport, 0, len(detail.Dimensions))
	for _, dim := range detail.Dimensions {
		dimensions = append(dimensions, reporttypology.MBTIDimensionReport{
			Code:       dim.Code,
			Name:       dim.Name,
			LeftPole:   dim.LeftPole,
			RightPole:  dim.RightPole,
			RawScore:   dim.RawScore,
			Preference: dim.Preference,
			Strength:   dim.Strength,
		})
	}
	return reporttypology.MBTIReportDetail{
		TypeCode:     detail.TypeCode,
		TypeName:     detail.TypeName,
		OneLiner:     detail.OneLiner,
		MatchPercent: detail.MatchPercent,
		ImageURL:     detail.ImageURL,
		Dimensions:   dimensions,
		Profile: reporttypology.MBTIProfileReport{
			TypeCode:    detail.Profile.TypeCode,
			TypeName:    detail.Profile.TypeName,
			OneLiner:    detail.Profile.OneLiner,
			Summary:     detail.Profile.Summary,
			Traits:      append([]string(nil), detail.Profile.Traits...),
			Strengths:   append([]string(nil), detail.Profile.Strengths...),
			Weaknesses:  append([]string(nil), detail.Profile.Weaknesses...),
			Suggestions: append([]string(nil), detail.Profile.Suggestions...),
			ImageURL:    detail.Profile.ImageURL,
		},
		Source: reporttypology.MBTISourceReport{
			QuestionsRepo: detail.Source.QuestionsRepo,
			SourceSite:    detail.Source.SourceSite,
			License:       detail.Source.License,
			Attribution:   detail.Source.Attribution,
			NonCommercial: detail.Source.NonCommercial,
		},
	}
}

func sbtiReportDetail(detail evaluationtypology.SBTIResultDetail) reporttypology.SBTIReportDetail {
	dimensions := make([]reporttypology.SBTIDimensionReport, 0, len(detail.Dimensions))
	for _, dim := range detail.Dimensions {
		dimensions = append(dimensions, reporttypology.SBTIDimensionReport{
			Code:     dim.Code,
			Name:     dim.Name,
			Model:    dim.Model,
			RawScore: dim.RawScore,
			Level:    dim.Level,
		})
	}
	return reporttypology.SBTIReportDetail{
		TypeCode:   detail.TypeCode,
		TypeName:   detail.TypeName,
		OneLiner:   detail.OneLiner,
		Pattern:    detail.Pattern,
		Similarity: detail.Similarity,
		ImageURL:   detail.ImageURL,
		Rarity: reporttypology.SBTIRarityReport{
			Percent: detail.Rarity.Percent,
			Label:   detail.Rarity.Label,
			OneInX:  detail.Rarity.OneInX,
		},
		Dimensions: dimensions,
		Outcome: reporttypology.SBTIOutcomeReport{
			Code:     detail.Outcome.Code,
			Name:     detail.Outcome.Name,
			OneLiner: detail.Outcome.OneLiner,
			Pattern:  detail.Outcome.Pattern,
			Image:    detail.Outcome.Image,
			Rarity: reporttypology.SBTIRarityReport{
				Percent: detail.Outcome.Rarity.Percent,
				Label:   detail.Outcome.Rarity.Label,
				OneInX:  detail.Outcome.Rarity.OneInX,
			},
			IsSpecial:  detail.Outcome.IsSpecial,
			Trigger:    detail.Outcome.Trigger,
			Commentary: detail.Outcome.Commentary,
		},
		Source: reporttypology.SBTISourceReport{
			WikiRepo:      detail.Source.WikiRepo,
			SourceSite:    detail.Source.SourceSite,
			License:       detail.Source.License,
			Attribution:   detail.Source.Attribution,
			ImageBaseURL:  detail.Source.ImageBaseURL,
			NonCommercial: detail.Source.NonCommercial,
		},
		SpecialTrigger: detail.SpecialTrigger,
	}
}
