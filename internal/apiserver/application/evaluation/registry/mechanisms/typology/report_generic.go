package typology

import (
	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	reporttypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/typology/patterns"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/typology"
)

func buildPersonalityTypeReport(_ modeltypology.ReportAdapterKey, outcome evaloutcome.Outcome) (*domainReport.InterpretReport, error) {
	return buildMechanismPersonalityTypeReport(outcome, legacyAlgorithmFromOutcome(outcome))
}

func buildTraitProfileReport(_ modeltypology.ReportAdapterKey, outcome evaloutcome.Outcome) (*domainReport.InterpretReport, error) {
	return buildMechanismTraitProfileReport(outcome, legacyAlgorithmFromOutcome(outcome))
}

func buildMechanismPersonalityTypeReport(outcome evaloutcome.Outcome, algorithm modelcatalog.Algorithm) (*domainReport.InterpretReport, error) {
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
		personalityTypeTemplateForAlgorithm(algorithm),
	)
}

func buildMechanismTraitProfileReport(outcome evaloutcome.Outcome, algorithm modelcatalog.Algorithm) (*domainReport.InterpretReport, error) {
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
		traitProfileTemplateForAlgorithm(algorithm),
	)
}
