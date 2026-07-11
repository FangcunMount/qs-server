package interpretation

import (
	"context"
	"errors"
	"fmt"
	"time"

	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	interpretationreporting "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/pkg/event"
)

type ReportStateStore interface {
	SaveState(ctx context.Context, report *domainreport.InterpretReport, testeeID testee.ID) error
	FindByID(ctx context.Context, id domainreport.ID) (*domainreport.InterpretReport, error)
}

type OutcomeReportService interface {
	GenerateByOutcomeID(ctx context.Context, outcomeID domainoutcome.ID) (*domainreport.InterpretReport, error)
	GenerateByAssessmentID(ctx context.Context, assessmentID meta.ID) (*domainreport.InterpretReport, error)
}

type outcomeReportService struct {
	outcomes     domainoutcome.Repository
	reports      ReportStateStore
	generator    interpretationreporting.Generator
	durableSaver interpretationreporting.ReportDurableSaver
	now          func() time.Time
}

func NewOutcomeReportService(outcomes domainoutcome.Repository, reports ReportStateStore, generator interpretationreporting.Generator, durableSaver interpretationreporting.ReportDurableSaver) OutcomeReportService {
	return &outcomeReportService{outcomes: outcomes, reports: reports, generator: generator, durableSaver: durableSaver, now: time.Now}
}

func (s *outcomeReportService) GenerateByAssessmentID(ctx context.Context, assessmentID meta.ID) (*domainreport.InterpretReport, error) {
	if s == nil || s.outcomes == nil {
		return nil, fmt.Errorf("evaluation outcome repository is not configured")
	}
	record, err := s.outcomes.FindByAssessmentID(ctx, assessmentID)
	if err != nil {
		return nil, err
	}
	return s.generate(ctx, record)
}

func (s *outcomeReportService) GenerateByOutcomeID(ctx context.Context, outcomeID domainoutcome.ID) (*domainreport.InterpretReport, error) {
	if s == nil || s.outcomes == nil {
		return nil, fmt.Errorf("evaluation outcome repository is not configured")
	}
	if outcomeID.IsZero() {
		return nil, fmt.Errorf("evaluation outcome id is required")
	}
	record, err := s.outcomes.FindByID(ctx, outcomeID)
	if err != nil {
		return nil, err
	}
	return s.generate(ctx, record)
}

func (s *outcomeReportService) generate(ctx context.Context, record *domainoutcome.Record) (*domainreport.InterpretReport, error) {
	if s.reports == nil || s.generator == nil || s.durableSaver == nil {
		return nil, fmt.Errorf("report lifecycle dependencies are not configured")
	}
	outcome, err := evaloutcome.Restore(record)
	if err != nil {
		return nil, err
	}
	now := s.now()
	rpt, err := s.reports.FindByID(ctx, domainreport.ID(record.AssessmentID()))
	newReport := false
	if errors.Is(err, domainreport.ErrReportNotFound) {
		rpt, err = domainreport.NewPendingInterpretReport(domainreport.ID(record.AssessmentID()), record.ID(), now)
		newReport = true
	}
	if err != nil {
		return nil, err
	}
	if rpt.OutcomeID() != record.ID() {
		if err := rpt.ResetForOutcome(record.ID(), now); err != nil {
			return rpt, err
		}
		newReport = true
	}
	if rpt.Status() == domainreport.ReportStatusGenerated {
		return rpt, nil
	}
	if newReport {
		if err := s.reports.SaveState(ctx, rpt, outcome.TesteeID()); err != nil {
			return rpt, err
		}
	}
	if err := rpt.BeginGenerating(now); err != nil {
		return rpt, err
	}
	if err := s.reports.SaveState(ctx, rpt, outcome.TesteeID()); err != nil {
		return rpt, err
	}

	generation, generationErr := s.generator.Generate(ctx, outcome)
	if generationErr != nil {
		return rpt, s.persistFailure(ctx, rpt, outcome, generationErr)
	}
	if err := rpt.CompleteFrom(generation.Report, s.now()); err != nil {
		return rpt, s.persistFailure(ctx, rpt, outcome, err)
	}
	if err := s.durableSaver.SaveReportDurably(ctx, rpt, outcome.TesteeID(), generation.Events); err != nil {
		return rpt, s.persistFailure(ctx, rpt, outcome, err)
	}
	return rpt, nil
}

func (s *outcomeReportService) persistFailure(ctx context.Context, rpt *domainreport.InterpretReport, outcome evaloutcome.Outcome, cause error) error {
	failedAt := s.now()
	if err := rpt.Fail(cause.Error(), failedAt); err != nil {
		return errors.Join(cause, err)
	}
	events := []event.DomainEvent{interpretationreporting.BuildReportFailedEvent(outcome, rpt, failedAt)}
	if err := s.durableSaver.SaveReportDurably(ctx, rpt, outcome.TesteeID(), events); err != nil {
		return errors.Join(cause, fmt.Errorf("persist report failure: %w", err))
	}
	return cause
}
