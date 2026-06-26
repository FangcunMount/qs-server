package result

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	evaluationtypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/personality/typology"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/report"
)

func modelIdentityFromOutcome(outcome Outcome) domainreport.ModelIdentity {
	if outcome.Execution != nil && !outcome.Execution.ModelRef.IsEmpty() {
		return modelIdentityFromRef(outcome.Execution.ModelRef)
	}
	if result := legacyResultForPersistence(outcome); result != nil && !result.ModelRef.IsEmpty() {
		return modelIdentityFromRef(result.ModelRef)
	}
	if outcome.Assessment != nil && outcome.Assessment.EvaluationModelRef() != nil {
		return modelIdentityFromRef(*outcome.Assessment.EvaluationModelRef())
	}
	if outcome.Input != nil && outcome.Input.Model != nil {
		return domainreport.ModelIdentity{
			Kind:      string(outcome.Input.Model.Kind),
			SubKind:   outcome.Input.Model.SubKind,
			Algorithm: outcome.Input.Model.Algorithm,
			Code:      outcome.Input.Model.Code,
			Version:   outcome.Input.Model.Version,
			Title:     outcome.Input.Model.Title,
		}
	}
	return domainreport.ModelIdentity{}
}

func modelIdentityFromRef(ref assessment.EvaluationModelRef) domainreport.ModelIdentity {
	identity := domainreport.ModelIdentity{
		Kind:      string(ref.Kind()),
		SubKind:   string(ref.SubKind()),
		Algorithm: string(ref.Algorithm()),
		Code:      ref.Code().String(),
		Version:   ref.Version(),
		Title:     ref.Title(),
	}
	if identity.Algorithm == "" {
		if mappedKind, subKind, algorithm, ok := assessmentmodel.LegacyKindMapping(assessmentmodel.Kind(ref.Kind())); ok {
			identity.Kind = string(mappedKind)
			identity.SubKind = string(subKind)
			identity.Algorithm = string(algorithm)
		}
	}
	return identity
}

func primaryScoreFromOutcome(outcome Outcome) *domainreport.ScoreValue {
	if outcome.Execution != nil && outcome.Execution.Primary != nil {
		return reportScoreFromOutcomeValue(outcome.Execution.Primary)
	}
	return primaryScoreFromLegacyResult(legacyResultForPersistence(outcome))
}

func reportScoreFromOutcomeValue(score *assessment.OutcomeScoreValue) *domainreport.ScoreValue {
	if score == nil {
		return nil
	}
	switch score.Kind {
	case assessment.OutcomeScoreKindMatchPercent:
		return domainreport.NewMatchPercentScore(score.Value, score.Label)
	case assessment.OutcomeScoreKindRawTotal:
		return domainreport.NewRawTotalScore(score.Value, score.Max)
	default:
		if score.Label != "" {
			return domainreport.NewMatchPercentScore(score.Value, score.Label)
		}
		return domainreport.NewRawTotalScore(score.Value, score.Max)
	}
}

func primaryScoreFromLegacyResult(result *assessment.EvaluationResult) *domainreport.ScoreValue {
	if result == nil {
		return nil
	}
	switch {
	case result.ModelRef.IsScale() || result.Detail.Kind == assessment.EvaluationModelKindScale:
		return domainreport.NewRawTotalScore(result.TotalScore, nil)
	case result.Detail.Kind == assessment.EvaluationModelKindPersonality:
		switch result.ModelRef.Algorithm() {
		case assessmentmodel.AlgorithmSBTI:
			if result.Summary.Score != nil {
				return domainreport.NewMatchPercentScore(*result.Summary.Score, result.Summary.PrimaryLabel)
			}
			if detail, err := evaluationtypology.SBTIResultDetailFromPayload(result.Detail.Payload); err == nil {
				return domainreport.NewMatchPercentScore(detail.Similarity*100, detail.TypeCode)
			}
		default:
			if detail, err := evaluationtypology.MBTIResultDetailFromPayload(result.Detail.Payload); err == nil {
				return domainreport.NewMatchPercentScore(detail.MatchPercent, detail.TypeCode)
			}
		}
	case result.Summary.Score != nil:
		return domainreport.NewMatchPercentScore(*result.Summary.Score, result.Summary.PrimaryLabel)
	}
	return nil
}

func levelFromOutcome(outcome Outcome) *domainreport.ResultLevel {
	if outcome.Execution != nil && outcome.Execution.Level != nil {
		return reportLevelFromOutcomeLevel(outcome.Execution.Level)
	}
	return levelFromLegacyResult(legacyResultForPersistence(outcome))
}

func reportLevelFromOutcomeLevel(level *assessment.OutcomeResultLevel) *domainreport.ResultLevel {
	if level == nil {
		return nil
	}
	if domainreport.IsRiskLevelCode(level.Code) {
		return domainreport.LevelFromRisk(domainreport.RiskLevel(level.Code))
	}
	return &domainreport.ResultLevel{
		Code:     level.Code,
		Label:    level.Label,
		Severity: level.Severity,
	}
}

func levelFromLegacyResult(result *assessment.EvaluationResult) *domainreport.ResultLevel {
	if result == nil {
		return nil
	}
	if result.RiskLevel != "" && result.RiskLevel != assessment.RiskLevelNone {
		return domainreport.LevelFromRisk(domainreport.RiskLevel(result.RiskLevel))
	}
	if result.Summary.Level != nil {
		level := domainreport.LevelFromRisk(domainreport.RiskLevel(*result.Summary.Level))
		if level != nil && result.Summary.PrimaryLabel != "" && result.Summary.PrimaryLabel != level.Code {
			level.Label = result.Summary.PrimaryLabel
		}
		return level
	}
	if result.Summary.PrimaryLabel != "" {
		return &domainreport.ResultLevel{
			Code:     result.Summary.PrimaryLabel,
			Label:    result.Summary.PrimaryLabel,
			Severity: "none",
		}
	}
	return domainreport.LevelFromRisk(domainreport.RiskLevelNone)
}

func AttachReportOutcomeSummary(outcome Outcome, report *domainreport.InterpretReport) *domainreport.InterpretReport {
	return attachOutcomeSummary(outcome, report)
}

func attachOutcomeSummary(outcome Outcome, report *domainreport.InterpretReport) *domainreport.InterpretReport {
	return domainreport.AttachOutcomeSummary(
		report,
		modelIdentityFromOutcome(outcome),
		primaryScoreFromOutcome(outcome),
		levelFromOutcome(outcome),
	)
}
