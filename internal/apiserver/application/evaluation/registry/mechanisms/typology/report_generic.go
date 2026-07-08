package typology

import (
	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	reporttypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/typology/patterns"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/typology"
)

func buildPersonalityTypeReport(_ modeltypology.ReportAdapterKey, outcome evaloutcome.Outcome) (*domainReport.InterpretReport, error) {
	spec, _, _ := resolveReportBuildContext(algorithmRunner{}, outcome)
	return buildMechanismPersonalityTypeReport(outcome, spec)
}

func buildTraitProfileReport(_ modeltypology.ReportAdapterKey, outcome evaloutcome.Outcome) (*domainReport.InterpretReport, error) {
	spec, _, _ := resolveReportBuildContext(algorithmRunner{}, outcome)
	return buildMechanismTraitProfileReport(outcome, spec)
}

func buildMechanismPersonalityTypeReport(outcome evaloutcome.Outcome, spec modeltypology.ReportSpec) (*domainReport.InterpretReport, error) {
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
		personalityTypeTemplateForSpec(spec),
	)
}

func buildMechanismTraitProfileReport(outcome evaloutcome.Outcome, spec modeltypology.ReportSpec) (*domainReport.InterpretReport, error) {
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
		traitProfileTemplateForSpec(spec),
	)
}
