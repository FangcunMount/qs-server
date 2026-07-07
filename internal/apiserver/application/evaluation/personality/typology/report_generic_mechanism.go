package typology

import (
	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	evaluationtypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/personality/typology"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	reporttypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/personality/typology"
)

func buildGenericPersonalityTypeReport(outcome evaloutcome.Outcome) (*domainReport.InterpretReport, error) {
	if outcome.Assessment == nil {
		return nil, errAssessmentRequired
	}
	if outcome.Execution == nil {
		return nil, errEvaluationOutcomeRequired
	}
	detail, err := personalityTypeDetailForReport(outcome.Execution.Detail.Payload)
	if err != nil {
		return nil, err
	}
	return reporttypology.BuildPersonalityTypeReport(
		reporttypology.PersonalityTypeReportInput{
			AssessmentID: domainReport.ID(outcome.Assessment.ID()),
			ModelCode:    typologyModelCode(outcome),
			TotalScore:   typologyTotalScore(outcome.Execution),
			RiskLevel:    typologyRiskLevel(outcome.Execution),
			Detail:       genericPersonalityTypeMechanismDetail(detail),
		},
		reporttypology.PersonalityTypeReportTemplate{},
	)
}

func buildGenericTraitProfileReport(outcome evaloutcome.Outcome) (*domainReport.InterpretReport, error) {
	if outcome.Assessment == nil {
		return nil, errAssessmentRequired
	}
	if outcome.Execution == nil {
		return nil, errEvaluationOutcomeRequired
	}
	detail, err := traitProfileDetailForReport(outcome.Execution.Detail.Payload)
	if err != nil {
		return nil, err
	}
	return reporttypology.BuildTraitProfileReport(
		reporttypology.TraitProfileReportInput{
			AssessmentID: domainReport.ID(outcome.Assessment.ID()),
			ModelCode:    typologyModelCode(outcome),
			TotalScore:   typologyTotalScore(outcome.Execution),
			RiskLevel:    typologyRiskLevel(outcome.Execution),
			Detail:       genericTraitProfileMechanismDetail(detail),
		},
		reporttypology.TraitProfileReportTemplate{},
	)
}

func genericPersonalityTypeMechanismDetail(detail evaluationtypology.PersonalityTypeDetail) reporttypology.PersonalityTypeReportDetail {
	dimensions := make([]reporttypology.PersonalityTypeDimensionReport, 0, len(detail.Dimensions))
	for _, dim := range detail.Dimensions {
		dimensions = append(dimensions, reporttypology.PersonalityTypeDimensionReport{
			Code: dim.Code, Name: dim.Name, LeftPole: dim.LeftPole, RightPole: dim.RightPole,
			RawScore: dim.RawScore, Preference: dim.Preference, Strength: dim.Strength,
			Model: dim.Model, Level: dim.Level,
		})
	}
	return reporttypology.PersonalityTypeReportDetail{
		TypeCode: detail.TypeCode, TypeName: detail.TypeName, OneLiner: detail.OneLiner,
		MatchPercent: detail.MatchPercent, ImageURL: detail.ImageURL,
		IsSpecial: detail.IsSpecial, SpecialTrigger: detail.SpecialTrigger, Commentary: detail.Commentary,
		Profile: reporttypology.PersonalityTypeProfileReport{
			Summary: detail.Summary, Strengths: append([]string(nil), detail.Strengths...),
			Weaknesses: append([]string(nil), detail.Weaknesses...), Suggestions: append([]string(nil), detail.Suggestions...),
		},
		Rarity: reporttypology.PersonalityTypeRarityReport{
			Percent: detail.Rarity.Percent, Label: detail.Rarity.Label, OneInX: detail.Rarity.OneInX,
		},
		Dimensions:        dimensions,
		SourceAttribution: detail.Source.Attribution, SourceLicense: detail.Source.License,
		SourceNonCommercial: detail.Source.NonCommercial,
	}
}

func genericTraitProfileMechanismDetail(detail evaluationtypology.TraitProfileDetail) reporttypology.TraitProfileReportDetail {
	traits := make([]reporttypology.TraitProfileFactorReport, 0, len(detail.Traits))
	for _, trait := range detail.Traits {
		traits = append(traits, reporttypology.TraitProfileFactorReport(trait))
	}
	return reporttypology.TraitProfileReportDetail{
		Traits: traits,
		Source: reporttypology.TraitProfileSourceReport{
			Attribution: detail.Source.Attribution, License: detail.Source.License,
			NonCommercial: detail.Source.NonCommercial,
		},
	}
}
