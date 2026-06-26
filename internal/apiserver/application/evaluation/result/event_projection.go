package result

import (
	"strconv"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/report"
	domainStatistics "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	"github.com/FangcunMount/qs-server/pkg/event"
)

func eventOutcomeFromReport(rpt *domainreport.InterpretReport, outcome Outcome) (
	assessment.EventModelIdentity,
	*assessment.EventScoreValue,
	*assessment.EventResultLevel,
) {
	model := modelIdentityFromOutcome(outcome)
	primary := primaryScoreFromOutcome(outcome)
	level := levelFromOutcome(outcome)
	if rpt != nil {
		if m := rpt.Model(); !m.IsEmpty() {
			model = m
		}
		if score := rpt.PrimaryScore(); score != nil {
			primary = score
		}
		if lv := rpt.Level(); lv != nil {
			level = lv
		}
	}
	return assessmentEventModelFrom(model),
		assessmentEventScoreFrom(primary),
		assessmentEventLevelFrom(level)
}

func assessmentEventModelFrom(model domainreport.ModelIdentity) assessment.EventModelIdentity {
	wire := domainreport.EventModelIdentityFrom(model)
	return assessment.EventModelIdentity{
		Kind:      wire.Kind,
		SubKind:   wire.SubKind,
		Algorithm: wire.Algorithm,
		Code:      wire.Code,
		Version:   wire.Version,
		Title:     wire.Title,
	}
}

func assessmentEventScoreFrom(score *domainreport.ScoreValue) *assessment.EventScoreValue {
	wire := domainreport.EventScoreValueFrom(score)
	if wire == nil {
		return nil
	}
	return &assessment.EventScoreValue{
		Kind:  wire.Kind,
		Value: wire.Value,
		Label: wire.Label,
		Max:   wire.Max,
	}
}

func assessmentEventLevelFrom(level *domainreport.ResultLevel) *assessment.EventResultLevel {
	wire := domainreport.EventResultLevelFrom(level)
	if wire == nil {
		return nil
	}
	return &assessment.EventResultLevel{
		Code:     wire.Code,
		Label:    wire.Label,
		Severity: wire.Severity,
	}
}

func buildInterpretedV2Event(outcome Outcome, rpt *domainreport.InterpretReport, at time.Time) event.DomainEvent {
	if outcome.Assessment == nil {
		return nil
	}
	model, primary, level := eventOutcomeFromReport(rpt, outcome)
	return assessment.NewAssessmentInterpretedV2Event(
		outcome.Assessment.OrgID(),
		outcome.Assessment.ID(),
		outcome.Assessment.TesteeID(),
		model,
		primary,
		level,
		at,
	)
}

func buildReportGeneratedV2Event(outcome Outcome, rpt *domainreport.InterpretReport, at time.Time) event.DomainEvent {
	if outcome.Assessment == nil || rpt == nil {
		return nil
	}
	model, primary, level := eventOutcomeFromReport(rpt, outcome)
	assessmentID := outcome.Assessment.ID().Uint64()
	reportID := rpt.ID().Uint64()
	return domainreport.NewReportGeneratedV2Event(
		strconv.FormatUint(reportID, 10),
		strconv.FormatUint(assessmentID, 10),
		outcome.Assessment.TesteeID().Uint64(),
		domainreport.EventModelIdentity{
			Kind: model.Kind, SubKind: model.SubKind, Algorithm: model.Algorithm,
			Code: model.Code, Version: model.Version, Title: model.Title,
		},
		reportEventScoreFrom(primary),
		reportEventLevelFrom(level),
		at,
	)
}

func reportEventScoreFrom(score *assessment.EventScoreValue) *domainreport.EventScoreValue {
	if score == nil {
		return nil
	}
	return &domainreport.EventScoreValue{
		Kind: score.Kind, Value: score.Value, Label: score.Label, Max: score.Max,
	}
}

func reportEventLevelFrom(level *assessment.EventResultLevel) *domainreport.EventResultLevel {
	if level == nil {
		return nil
	}
	return &domainreport.EventResultLevel{
		Code: level.Code, Label: level.Label, Severity: level.Severity,
	}
}

func buildFootprintReportGeneratedEvent(outcome Outcome, rpt *domainreport.InterpretReport, at time.Time) event.DomainEvent {
	if outcome.Assessment == nil || rpt == nil {
		return nil
	}
	return domainStatistics.NewFootprintReportGeneratedEvent(
		outcome.Assessment.OrgID(),
		outcome.Assessment.TesteeID().Uint64(),
		outcome.Assessment.ID().Uint64(),
		rpt.ID().Uint64(),
		at,
	)
}
