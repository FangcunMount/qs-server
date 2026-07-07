package factor_classification

import (
	"fmt"

	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	evaluationtypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/personality/typology"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	reporttypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/personality/typology"
)

var (
	errAssessmentRequired        = fmt.Errorf("assessment is required")
	errEvaluationOutcomeRequired = fmt.Errorf("evaluation outcome is required")
)

func personalityTypeDetailForReport(payload any) (evaluationtypology.PersonalityTypeDetail, error) {
	if detail, err := evaluationtypology.PersonalityTypeDetailFromPayload(payload); err == nil {
		return detail, nil
	}
	if detail, err := evaluationtypology.MBTIResultDetailFromPayload(payload); err == nil {
		return evaluationtypology.PersonalityTypeDetailFromMBTI(detail), nil
	}
	if detail, err := evaluationtypology.SBTIResultDetailFromPayload(payload); err == nil {
		return evaluationtypology.PersonalityTypeDetailFromSBTI(detail), nil
	}
	return evaluationtypology.PersonalityTypeDetail{}, fmt.Errorf("unsupported personality type detail payload")
}

func traitProfileDetailForReport(payload any) (evaluationtypology.TraitProfileDetail, error) {
	if detail, err := evaluationtypology.TraitProfileDetailFromPayload(payload); err == nil {
		return detail, nil
	}
	if detail, err := evaluationtypology.BigFiveResultDetailFromPayload(payload); err == nil {
		return evaluationtypology.TraitProfileDetailFromBigFive(detail), nil
	}
	return evaluationtypology.TraitProfileDetail{}, fmt.Errorf("unsupported trait profile detail payload")
}

func MBTIReportInputFromOutcome(outcome evaloutcome.Outcome) (reporttypology.MBTIReportInput, error) {
	if outcome.Assessment == nil {
		return reporttypology.MBTIReportInput{}, errAssessmentRequired
	}
	if outcome.Execution == nil {
		return reporttypology.MBTIReportInput{}, errEvaluationOutcomeRequired
	}
	payload := outcome.Execution.Detail.Payload
	detail, err := evaluationtypology.MBTIResultDetailFromPayload(payload)
	if err != nil {
		generic, gerr := personalityTypeDetailForReport(payload)
		if gerr != nil {
			return reporttypology.MBTIReportInput{}, err
		}
		return reporttypology.MBTIReportInput{
			AssessmentID: domainReport.ID(outcome.Assessment.ID()),
			ModelCode:    typologyModelCode(outcome),
			TotalScore:   typologyTotalScore(outcome.Execution),
			RiskLevel:    typologyRiskLevel(outcome.Execution),
			Detail:       mbtiReportDetailFromPersonalityType(generic),
		}, nil
	}
	return reporttypology.MBTIReportInput{
		AssessmentID: domainReport.ID(outcome.Assessment.ID()),
		ModelCode:    typologyModelCode(outcome),
		TotalScore:   typologyTotalScore(outcome.Execution),
		RiskLevel:    typologyRiskLevel(outcome.Execution),
		Detail:       mbtiReportDetail(detail),
	}, nil
}

func BigFiveReportInputFromOutcome(outcome evaloutcome.Outcome) (reporttypology.BigFiveReportInput, error) {
	if outcome.Assessment == nil {
		return reporttypology.BigFiveReportInput{}, errAssessmentRequired
	}
	if outcome.Execution == nil {
		return reporttypology.BigFiveReportInput{}, errEvaluationOutcomeRequired
	}
	payload := outcome.Execution.Detail.Payload
	detail, err := evaluationtypology.BigFiveResultDetailFromPayload(payload)
	if err != nil {
		generic, gerr := traitProfileDetailForReport(payload)
		if gerr != nil {
			return reporttypology.BigFiveReportInput{}, err
		}
		return reporttypology.BigFiveReportInput{
			AssessmentID: domainReport.ID(outcome.Assessment.ID()),
			ModelCode:    typologyModelCode(outcome),
			TotalScore:   typologyTotalScore(outcome.Execution),
			RiskLevel:    typologyRiskLevel(outcome.Execution),
			Detail:       bigFiveReportDetailFromTraitProfile(generic),
		}, nil
	}
	return reporttypology.BigFiveReportInput{
		AssessmentID: domainReport.ID(outcome.Assessment.ID()),
		ModelCode:    typologyModelCode(outcome),
		TotalScore:   typologyTotalScore(outcome.Execution),
		RiskLevel:    typologyRiskLevel(outcome.Execution),
		Detail:       bigFiveReportDetail(detail),
	}, nil
}

func SBTIReportInputFromOutcome(outcome evaloutcome.Outcome) (reporttypology.SBTIReportInput, error) {
	if outcome.Assessment == nil {
		return reporttypology.SBTIReportInput{}, errAssessmentRequired
	}
	if outcome.Execution == nil {
		return reporttypology.SBTIReportInput{}, errEvaluationOutcomeRequired
	}
	payload := outcome.Execution.Detail.Payload
	detail, err := evaluationtypology.SBTIResultDetailFromPayload(payload)
	if err != nil {
		generic, gerr := personalityTypeDetailForReport(payload)
		if gerr != nil {
			return reporttypology.SBTIReportInput{}, err
		}
		return reporttypology.SBTIReportInput{
			AssessmentID: domainReport.ID(outcome.Assessment.ID()),
			ModelCode:    typologyModelCode(outcome),
			TotalScore:   typologyTotalScore(outcome.Execution),
			RiskLevel:    typologyRiskLevel(outcome.Execution),
			Detail:       sbtiReportDetailFromPersonalityType(generic),
		}, nil
	}
	return reporttypology.SBTIReportInput{
		AssessmentID: domainReport.ID(outcome.Assessment.ID()),
		ModelCode:    typologyModelCode(outcome),
		TotalScore:   typologyTotalScore(outcome.Execution),
		RiskLevel:    typologyRiskLevel(outcome.Execution),
		Detail:       sbtiReportDetail(detail),
	}, nil
}

func typologyModelCode(outcome evaloutcome.Outcome) string {
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

func bigFiveReportDetail(detail evaluationtypology.BigFiveResultDetail) reporttypology.BigFiveReportDetail {
	traits := make([]reporttypology.BigFiveTraitReport, 0, len(detail.Traits))
	for _, trait := range detail.Traits {
		traits = append(traits, reporttypology.BigFiveTraitReport{
			Code:     trait.Code,
			Name:     trait.Name,
			RawScore: trait.RawScore,
		})
	}
	return reporttypology.BigFiveReportDetail{
		Traits: traits,
		Source: reporttypology.BigFiveSourceReport{
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

func mbtiReportDetailFromPersonalityType(detail evaluationtypology.PersonalityTypeDetail) reporttypology.MBTIReportDetail {
	dimensions := make([]reporttypology.MBTIDimensionReport, 0, len(detail.Dimensions))
	for _, dim := range detail.Dimensions {
		dimensions = append(dimensions, reporttypology.MBTIDimensionReport{
			Code: dim.Code, Name: dim.Name, LeftPole: dim.LeftPole, RightPole: dim.RightPole,
			RawScore: dim.RawScore, Preference: dim.Preference, Strength: dim.Strength,
		})
	}
	return reporttypology.MBTIReportDetail{
		TypeCode: detail.TypeCode, TypeName: detail.TypeName, OneLiner: detail.OneLiner,
		MatchPercent: detail.MatchPercent, ImageURL: detail.ImageURL, Dimensions: dimensions,
		Profile: reporttypology.MBTIProfileReport{
			TypeCode: detail.TypeCode, TypeName: detail.TypeName, OneLiner: detail.OneLiner,
			Summary: detail.Summary, Strengths: append([]string(nil), detail.Strengths...),
			Weaknesses: append([]string(nil), detail.Weaknesses...), Suggestions: append([]string(nil), detail.Suggestions...),
			ImageURL: detail.ImageURL,
		},
		Source: reporttypology.MBTISourceReport{
			Attribution: detail.Source.Attribution, License: detail.Source.License,
			NonCommercial: detail.Source.NonCommercial,
		},
	}
}

func bigFiveReportDetailFromTraitProfile(detail evaluationtypology.TraitProfileDetail) reporttypology.BigFiveReportDetail {
	traits := make([]reporttypology.BigFiveTraitReport, 0, len(detail.Traits))
	for _, trait := range detail.Traits {
		traits = append(traits, reporttypology.BigFiveTraitReport(trait))
	}
	return reporttypology.BigFiveReportDetail{
		Traits: traits,
		Source: reporttypology.BigFiveSourceReport{
			Attribution: detail.Source.Attribution, License: detail.Source.License,
			NonCommercial: detail.Source.NonCommercial,
		},
	}
}

func sbtiReportDetailFromPersonalityType(detail evaluationtypology.PersonalityTypeDetail) reporttypology.SBTIReportDetail {
	dimensions := make([]reporttypology.SBTIDimensionReport, 0, len(detail.Dimensions))
	for _, dim := range detail.Dimensions {
		dimensions = append(dimensions, reporttypology.SBTIDimensionReport{
			Code: dim.Code, Name: dim.Name, Model: dim.Model, RawScore: dim.RawScore, Level: dim.Level,
		})
	}
	similarity := detail.Similarity
	if similarity == 0 && detail.MatchPercent > 0 {
		similarity = detail.MatchPercent / 100
	}
	return reporttypology.SBTIReportDetail{
		TypeCode: detail.TypeCode, TypeName: detail.TypeName, OneLiner: detail.OneLiner,
		Pattern: detail.Pattern, Similarity: similarity, ImageURL: detail.ImageURL,
		Rarity: reporttypology.SBTIRarityReport{
			Percent: detail.Rarity.Percent, Label: detail.Rarity.Label, OneInX: detail.Rarity.OneInX,
		},
		Dimensions: dimensions,
		Outcome: reporttypology.SBTIOutcomeReport{
			Code: detail.Outcome.Code, Name: detail.Outcome.Name, OneLiner: detail.Outcome.OneLiner,
			Pattern: detail.Outcome.Pattern, Image: detail.Outcome.Image,
			Rarity: reporttypology.SBTIRarityReport{
				Percent: detail.Outcome.Rarity.Percent, Label: detail.Outcome.Rarity.Label, OneInX: detail.Outcome.Rarity.OneInX,
			},
			IsSpecial: detail.Outcome.IsSpecial, Trigger: detail.Outcome.Trigger, Commentary: detail.Outcome.Commentary,
		},
		Source: reporttypology.SBTISourceReport{
			WikiRepo: detail.Source.WikiRepo, SourceSite: detail.Source.SourceSite,
			License: detail.Source.License, Attribution: detail.Source.Attribution,
			ImageBaseURL: detail.Source.ImageBaseURL, NonCommercial: detail.Source.NonCommercial,
		},
		SpecialTrigger: detail.SpecialTrigger,
	}
}
