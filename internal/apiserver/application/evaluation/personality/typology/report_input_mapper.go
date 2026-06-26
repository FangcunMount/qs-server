package typology

import (
	evaluationresult "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/result"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	evaluationtypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/personality/typology"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/report"
	reporttypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/report/personality/typology"
)

func MBTIReportInputFromOutcome(outcome evaluationresult.Outcome) (reporttypology.MBTIReportInput, error) {
	if outcome.Assessment == nil {
		return reporttypology.MBTIReportInput{}, errAssessmentRequired
	}
	if outcome.Execution == nil {
		return reporttypology.MBTIReportInput{}, errEvaluationOutcomeRequired
	}
	detail, err := evaluationtypology.MBTIResultDetailFromPayload(outcome.Execution.Detail.Payload)
	if err != nil {
		return reporttypology.MBTIReportInput{}, err
	}
	return reporttypology.MBTIReportInput{
		AssessmentID: domainReport.ID(outcome.Assessment.ID()),
		ModelCode:    typologyModelCode(outcome),
		TotalScore:   typologyTotalScore(outcome.Execution),
		RiskLevel:    typologyRiskLevel(outcome.Execution),
		Detail:       mbtiReportDetail(detail),
	}, nil
}

func SBTIReportInputFromOutcome(outcome evaluationresult.Outcome) (reporttypology.SBTIReportInput, error) {
	if outcome.Assessment == nil {
		return reporttypology.SBTIReportInput{}, errAssessmentRequired
	}
	if outcome.Execution == nil {
		return reporttypology.SBTIReportInput{}, errEvaluationOutcomeRequired
	}
	detail, err := evaluationtypology.SBTIResultDetailFromPayload(outcome.Execution.Detail.Payload)
	if err != nil {
		return reporttypology.SBTIReportInput{}, err
	}
	return reporttypology.SBTIReportInput{
		AssessmentID: domainReport.ID(outcome.Assessment.ID()),
		ModelCode:    typologyModelCode(outcome),
		TotalScore:   typologyTotalScore(outcome.Execution),
		RiskLevel:    typologyRiskLevel(outcome.Execution),
		Detail:       sbtiReportDetail(detail),
	}, nil
}

func typologyModelCode(outcome evaluationresult.Outcome) string {
	if outcome.Execution != nil && !outcome.Execution.ModelRef.Code().IsEmpty() {
		return outcome.Execution.ModelRef.Code().String()
	}
	return ""
}

func typologyTotalScore(execution *assessment.AssessmentOutcome) float64 {
	if execution == nil || execution.Primary == nil {
		return 0
	}
	return execution.Primary.Value
}

func typologyRiskLevel(execution *assessment.AssessmentOutcome) domainReport.RiskLevel {
	if execution == nil || execution.Level == nil {
		return domainReport.RiskLevelNone
	}
	return domainReport.RiskLevel(execution.Level.Code)
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
