package result

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/report"
)

func modelIdentityFromOutcome(outcome Outcome) domainreport.ModelIdentity {
	if outcome.Execution != nil && !outcome.Execution.ModelRef.IsEmpty() {
		return modelIdentityFromRef(outcome.Execution.ModelRef)
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
	if outcome.Execution == nil {
		return nil
	}
	if outcome.Execution.Primary != nil {
		return reportScoreFromOutcomeValue(outcome.Execution.Primary)
	}
	if outcome.Execution.Summary.Score != nil {
		return domainreport.NewMatchPercentScore(*outcome.Execution.Summary.Score, outcome.Execution.Summary.PrimaryLabel)
	}
	return nil
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

func levelFromOutcome(outcome Outcome) *domainreport.ResultLevel {
	if outcome.Execution == nil {
		return nil
	}
	if outcome.Execution.Level != nil {
		return reportLevelFromOutcomeLevel(outcome.Execution.Level)
	}
	if outcome.Execution.Summary.Level != nil {
		level := domainreport.LevelFromRisk(domainreport.RiskLevel(*outcome.Execution.Summary.Level))
		if level != nil && outcome.Execution.Summary.PrimaryLabel != "" && outcome.Execution.Summary.PrimaryLabel != level.Code {
			level.Label = outcome.Execution.Summary.PrimaryLabel
		}
		return level
	}
	if outcome.Execution.Summary.PrimaryLabel != "" {
		return &domainreport.ResultLevel{
			Code:     outcome.Execution.Summary.PrimaryLabel,
			Label:    outcome.Execution.Summary.PrimaryLabel,
			Severity: "none",
		}
	}
	return domainreport.LevelFromRisk(domainreport.RiskLevelNone)
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
