package result

import (
	"strconv"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	domainStatistics "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	"github.com/FangcunMount/qs-server/internal/pkg/eventoutcome"
	"github.com/FangcunMount/qs-server/pkg/event"
)

func eventOutcomeFromReport(rpt *domainreport.InterpretReport, outcome Outcome) (
	eventoutcome.ModelIdentity,
	*eventoutcome.ScoreValue,
	*eventoutcome.ResultLevel,
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
	return eventModelFrom(model),
		eventScoreFrom(primary),
		eventLevelFrom(level)
}

func eventModelFrom(model domainreport.ModelIdentity) eventoutcome.ModelIdentity {
	wire := domainreport.EventModelIdentityFrom(model)
	return eventoutcome.ModelIdentity(wire)
}

func eventScoreFrom(score *domainreport.ScoreValue) *eventoutcome.ScoreValue {
	wire := domainreport.EventScoreValueFrom(score)
	if wire == nil {
		return nil
	}
	return &eventoutcome.ScoreValue{
		Kind:  wire.Kind,
		Value: wire.Value,
		Label: wire.Label,
		Max:   wire.Max,
	}
}

func eventLevelFrom(level *domainreport.ResultLevel) *eventoutcome.ResultLevel {
	wire := domainreport.EventResultLevelFrom(level)
	if wire == nil {
		return nil
	}
	return &eventoutcome.ResultLevel{
		Code:     wire.Code,
		Label:    wire.Label,
		Severity: wire.Severity,
	}
}

func buildInterpretedOutcomeEvent(outcome Outcome, rpt *domainreport.InterpretReport, at time.Time) event.DomainEvent {
	if outcome.Assessment == nil {
		return nil
	}
	model, primary, level := eventOutcomeFromReport(rpt, outcome)
	return assessment.NewAssessmentInterpretedOutcomeEvent(
		outcome.Assessment.OrgID(),
		outcome.Assessment.ID(),
		outcome.Assessment.TesteeID(),
		model,
		primary,
		level,
		at,
	)
}

func buildReportGeneratedOutcomeEvent(outcome Outcome, rpt *domainreport.InterpretReport, at time.Time) event.DomainEvent {
	if outcome.Assessment == nil || rpt == nil {
		return nil
	}
	model, primary, level := eventOutcomeFromReport(rpt, outcome)
	assessmentID := outcome.Assessment.ID().Uint64()
	reportID := rpt.ID().Uint64()
	return domainreport.NewReportGeneratedOutcomeEvent(
		strconv.FormatUint(reportID, 10),
		strconv.FormatUint(assessmentID, 10),
		outcome.Assessment.TesteeID().Uint64(),
		model,
		primary,
		level,
		at,
	)
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
