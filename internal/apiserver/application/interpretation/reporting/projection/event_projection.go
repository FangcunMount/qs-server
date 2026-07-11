package projection

import (
	"strconv"
	"time"

	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/pkg/eventoutcome"
	"github.com/FangcunMount/qs-server/internal/pkg/footprintevent"
	"github.com/FangcunMount/qs-server/pkg/event"
)

func eventOutcomeFromReport(rpt *domainreport.InterpretReport, outcome evaloutcome.Outcome) (
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

func buildReportGeneratedOutcomeEvent(outcome evaloutcome.Outcome, rpt *domainreport.InterpretReport, at time.Time) event.DomainEvent {
	if outcome.Assessment == nil || rpt == nil {
		return nil
	}
	assessmentID := outcome.Assessment.ID().Uint64()
	reportID := rpt.ID().Uint64()
	return domainreport.NewInterpretationReportGeneratedEvent(domainreport.ReportGeneratedEventInput{
		OrgID: outcome.Assessment.OrgID(), GenerationID: strconv.FormatUint(reportID, 10), RunID: strconv.FormatUint(reportID, 10), ReportID: strconv.FormatUint(reportID, 10),
		AssessmentID: strconv.FormatUint(assessmentID, 10), OutcomeID: rpt.OutcomeID().String(), TesteeID: outcome.Assessment.TesteeID().Uint64(), Attempt: rpt.Attempt(),
		ReportType: "standard", TemplateVersion: policy.TemplateVersionV1.String(), BuilderIdentity: "legacy-projection", ContentSchemaVersion: "legacy-v1",
		Model: domainreport.EventModelIdentityFrom(rpt.Model()), PrimaryScore: domainreport.EventScoreValueFrom(rpt.PrimaryScore()), Level: domainreport.EventResultLevelFrom(rpt.Level()), GeneratedAt: at,
	})
}

// BuildReportFailedEvent creates the durable external fact for one failed
// Interpretation attempt. It deliberately consumes Evaluation only as input;
// it does not mutate Assessment, EvaluationRun, or EvaluationOutcome.
func BuildReportFailedEvent(outcome evaloutcome.Outcome, rpt *domainreport.InterpretReport, at time.Time) event.DomainEvent {
	if outcome.Assessment == nil || rpt == nil {
		return nil
	}
	return domainreport.NewInterpretationReportFailedEvent(domainreport.ReportFailedEventInput{
		OrgID: outcome.Assessment.OrgID(), GenerationID: rpt.ID().String(), RunID: rpt.ID().String(), AssessmentID: outcome.Assessment.ID().String(),
		OutcomeID: rpt.OutcomeID().String(), TesteeID: outcome.Assessment.TesteeID().Uint64(), Attempt: rpt.Attempt(), ReportType: "standard",
		TemplateVersion: policy.TemplateVersionV1.String(), FailureKind: "legacy", FailureCode: "legacy_report_failed", Retryable: false, SafeReason: rpt.FailureReason(), FailedAt: at,
	})
}

func buildFootprintReportGeneratedEvent(outcome evaloutcome.Outcome, rpt *domainreport.InterpretReport, at time.Time) event.DomainEvent {
	if outcome.Assessment == nil || rpt == nil {
		return nil
	}
	return footprintevent.NewFootprintReportGeneratedEvent(
		outcome.Assessment.OrgID(),
		outcome.Assessment.TesteeID().Uint64(),
		outcome.Assessment.ID().Uint64(),
		rpt.ID().Uint64(),
		at,
	)
}
