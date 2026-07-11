package typology

import (
	"fmt"

	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationcompat"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact"
	outcometypology "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationtypology"
)

var (
	errAssessmentRequired        = fmt.Errorf("assessment is required")
	errEvaluationOutcomeRequired = fmt.Errorf("evaluation outcome is required")
)

func personalityTypeDetailForReport(payload any) (outcometypology.PersonalityTypeDetail, error) {
	return personalityTypeDetailFromLegacyPayload(payload)
}

func traitProfileDetailForReport(payload any) (outcometypology.TraitProfileDetail, error) {
	return traitProfileDetailFromLegacyPayload(payload)
}

func typologyModelCode(outcome evaloutcome.Outcome) string {
	if outcome.Execution != nil && !outcome.Execution.ModelRef.Code().IsEmpty() {
		return outcome.Execution.ModelRef.Code().String()
	}
	return ""
}

func typologyTotalScore(execution *domainoutcome.Execution) float64 {
	if execution == nil || execution.Primary == nil {
		return 0
	}
	return execution.Primary.Value
}

func typologyRiskLevel(execution *domainoutcome.Execution) domainReport.RiskLevel {
	if execution == nil || execution.Level == nil {
		return domainReport.RiskLevelNone
	}
	return domainReport.RiskLevel(execution.Level.Code)
}
