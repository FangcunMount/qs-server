package evaluation

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func applyAssessmentOutcomeV2Fields(po *AssessmentPO, a *assessment.Assessment) {
	if po == nil || a == nil {
		return
	}
	if ref := a.EvaluationModelRef(); ref != nil && !ref.IsEmpty() {
		id := ref.ExecutionIdentity()
		subKind, algorithm := id.SubKind, id.Algorithm
		if subKind != "" {
			po.EvaluationModelSubKind = strPtr(string(subKind))
		}
		if algorithm != "" {
			po.EvaluationModelAlgorithm = strPtr(string(algorithm))
		}
	}
	if a.Status().IsEvaluated() {
		applyPrimaryScoreFields(po, a)
		applyLevelFields(po, a)
	}
}

func applyPrimaryScoreFields(po *AssessmentPO, a *assessment.Assessment) {
	if summary := a.ResultSummary(); summary != nil && summary.Score != nil {
		label := summary.PrimaryLabel
		if ref := a.EvaluationModelRef(); ref != nil && ref.Kind() == assessment.EvaluationModelKindTypology {
			po.PrimaryScoreKind = strPtr(string(domainoutcome.ScoreKindMatchPercent))
			po.PrimaryScoreValue = summary.Score
			if label != "" {
				po.PrimaryScoreLabel = strPtr(label)
			}
			return
		}
	}
	if total := a.TotalScore(); total != nil {
		po.PrimaryScoreKind = strPtr(string(domainoutcome.ScoreKindRawTotal))
		po.PrimaryScoreValue = total
	}
}

func applyLevelFields(po *AssessmentPO, a *assessment.Assessment) {
	if risk := a.RiskLevel(); risk != nil && *risk != "" && *risk != assessment.RiskLevelNone {
		po.LevelCode = strPtr(string(*risk))
		po.LevelLabel = strPtr(string(*risk))
		po.Severity = strPtr(evaluationRiskSeverity(*risk))
		return
	}
	if summary := a.ResultSummary(); summary != nil && summary.PrimaryLabel != "" {
		po.LevelCode = strPtr(summary.PrimaryLabel)
		po.LevelLabel = strPtr(summary.PrimaryLabel)
		po.Severity = strPtr("none")
	}
}

func evaluationRiskSeverity(risk assessment.RiskLevel) string {
	switch risk {
	case assessment.RiskLevelSevere, assessment.RiskLevelHigh:
		return "high"
	case assessment.RiskLevelMedium:
		return "medium"
	case assessment.RiskLevelLow:
		return "low"
	default:
		return "none"
	}
}

func strPtr(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

func subKindFromPO(po *AssessmentPO) modelcatalog.SubKind {
	if po == nil || po.EvaluationModelSubKind == nil {
		return modelcatalog.SubKindEmpty
	}
	return modelcatalog.SubKind(*po.EvaluationModelSubKind)
}

func algorithmFromPO(po *AssessmentPO) modelcatalog.Algorithm {
	if po == nil || po.EvaluationModelAlgorithm == nil {
		return ""
	}
	return modelcatalog.Algorithm(*po.EvaluationModelAlgorithm)
}
