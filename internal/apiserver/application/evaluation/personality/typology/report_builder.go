package typology

import (
	"context"
	"fmt"

	evaluationresult "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/result"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	evaluationtypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/personality/typology"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/report"
	reportmbti "github.com/FangcunMount/qs-server/internal/apiserver/domain/report/mbti"
	reportsbti "github.com/FangcunMount/qs-server/internal/apiserver/domain/report/sbti"
)

type ReportBuilder struct{}

var _ evaluationresult.ReportBuilder = ReportBuilder{}

func NewReportBuilder() ReportBuilder {
	return ReportBuilder{}
}

func (ReportBuilder) Kind() assessment.EvaluationModelKind {
	return assessment.EvaluationModelKindPersonality
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
	algorithm := outcome.Result.ModelRef.Algorithm()
	if algorithm == "" {
		switch outcome.Result.Detail.Payload.(type) {
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
	detail, err := evaluationtypology.MBTIResultDetailFromPayload(outcome.Result.Detail.Payload)
	if err != nil {
		return nil, err
	}
	modelCode := ""
	if !outcome.Result.ModelRef.Code().IsEmpty() {
		modelCode = outcome.Result.ModelRef.Code().String()
	}
	rpt, err := reportmbti.BuildReport(reportmbti.ReportInput{
		AssessmentID: domainReport.ID(outcome.Assessment.ID()),
		ModelCode:    modelCode,
		TotalScore:   outcome.Result.TotalScore,
		RiskLevel:    domainReport.RiskLevel(outcome.Result.RiskLevel),
		Detail:       mbtiReportDetail(detail),
	})
	if err != nil {
		return nil, err
	}
	return evaluationresult.AttachReportOutcomeSummary(outcome, rpt), nil
}

func buildSBTIReport(outcome evaluationresult.Outcome) (*domainReport.InterpretReport, error) {
	detail, err := evaluationtypology.SBTIResultDetailFromPayload(outcome.Result.Detail.Payload)
	if err != nil {
		return nil, err
	}
	modelCode := ""
	if !outcome.Result.ModelRef.Code().IsEmpty() {
		modelCode = outcome.Result.ModelRef.Code().String()
	}
	rpt, err := reportsbti.BuildReport(reportsbti.ReportInput{
		AssessmentID: domainReport.ID(outcome.Assessment.ID()),
		ModelCode:    modelCode,
		TotalScore:   outcome.Result.TotalScore,
		RiskLevel:    domainReport.RiskLevel(outcome.Result.RiskLevel),
		Detail:       sbtiReportDetail(detail),
	})
	if err != nil {
		return nil, err
	}
	return evaluationresult.AttachReportOutcomeSummary(outcome, rpt), nil
}

func mbtiReportDetail(detail evaluationtypology.MBTIResultDetail) reportmbti.ReportDetail {
	dimensions := make([]reportmbti.DimensionReport, 0, len(detail.Dimensions))
	for _, dim := range detail.Dimensions {
		dimensions = append(dimensions, reportmbti.DimensionReport{
			Code:       dim.Code,
			Name:       dim.Name,
			LeftPole:   dim.LeftPole,
			RightPole:  dim.RightPole,
			RawScore:   dim.RawScore,
			Preference: dim.Preference,
			Strength:   dim.Strength,
		})
	}
	return reportmbti.ReportDetail{
		TypeCode:     detail.TypeCode,
		TypeName:     detail.TypeName,
		OneLiner:     detail.OneLiner,
		MatchPercent: detail.MatchPercent,
		ImageURL:     detail.ImageURL,
		Dimensions:   dimensions,
		Profile: reportmbti.ProfileReport{
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
		Source: reportmbti.SourceReport{
			QuestionsRepo: detail.Source.QuestionsRepo,
			SourceSite:    detail.Source.SourceSite,
			License:       detail.Source.License,
			Attribution:   detail.Source.Attribution,
			NonCommercial: detail.Source.NonCommercial,
		},
	}
}

func sbtiReportDetail(detail evaluationtypology.SBTIResultDetail) reportsbti.ReportDetail {
	dimensions := make([]reportsbti.DimensionReport, 0, len(detail.Dimensions))
	for _, dim := range detail.Dimensions {
		dimensions = append(dimensions, reportsbti.DimensionReport{
			Code:     dim.Code,
			Name:     dim.Name,
			Model:    dim.Model,
			RawScore: dim.RawScore,
			Level:    dim.Level,
		})
	}
	return reportsbti.ReportDetail{
		TypeCode:   detail.TypeCode,
		TypeName:   detail.TypeName,
		OneLiner:   detail.OneLiner,
		Pattern:    detail.Pattern,
		Similarity: detail.Similarity,
		ImageURL:   detail.ImageURL,
		Rarity: reportsbti.RarityReport{
			Percent: detail.Rarity.Percent,
			Label:   detail.Rarity.Label,
			OneInX:  detail.Rarity.OneInX,
		},
		Dimensions: dimensions,
		Outcome: reportsbti.OutcomeReport{
			Code:     detail.Outcome.Code,
			Name:     detail.Outcome.Name,
			OneLiner: detail.Outcome.OneLiner,
			Pattern:  detail.Outcome.Pattern,
			Image:    detail.Outcome.Image,
			Rarity: reportsbti.RarityReport{
				Percent: detail.Outcome.Rarity.Percent,
				Label:   detail.Outcome.Rarity.Label,
				OneInX:  detail.Outcome.Rarity.OneInX,
			},
			IsSpecial:  detail.Outcome.IsSpecial,
			Trigger:    detail.Outcome.Trigger,
			Commentary: detail.Outcome.Commentary,
		},
		Source: reportsbti.SourceReport{
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
