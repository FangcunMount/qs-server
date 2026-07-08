package typology

import (
	"fmt"

	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	typologylegacy "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/typology/legacy"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	evaluationtypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/typology/patterns"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
)

var (
	errAssessmentRequired        = fmt.Errorf("assessment is required")
	errEvaluationOutcomeRequired = fmt.Errorf("evaluation outcome is required")
)

func personalityTypeDetailForReport(payload any) (evaluationtypology.PersonalityTypeDetail, error) {
	return typologylegacy.PersonalityTypeDetailForReport(payload)
}

func traitProfileDetailForReport(payload any) (evaluationtypology.TraitProfileDetail, error) {
	return typologylegacy.TraitProfileDetailForReport(payload)
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
