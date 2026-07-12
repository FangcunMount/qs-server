// Package operations exposes Interpretation lifecycle diagnostics to
// explicitly authorized operations and audit actors.
package operations

import (
	"context"
	"fmt"
	cberrors "github.com/FangcunMount/component-base/pkg/errors"
	domaingeneration "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/generation"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	domainrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/run"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"time"
)

const PermissionAudit = "interpretation.audit"

type Actor struct {
	OrgID, OperatorUserID int64
	Permissions           []string
}
type OutcomeCorrelation interface {
	FindOutcomeIDByAssessmentID(context.Context, meta.ID) (meta.ID, error)
}
type Generation struct {
	ID, OutcomeID, LatestRunID, ReportID uint64
	ReportType, TemplateVersion, Status  string
	Version                              uint64
	CreatedAt, UpdatedAt                 time.Time
	LatestRun                            *Run
	Report                               *Report
}
type Run struct {
	ID, GenerationID                      uint64
	Attempt                               int
	Status, TraceID                       string
	Failure                               *Failure
	StartedAt, LeaseExpiresAt, FinishedAt *time.Time
}
type Failure struct {
	Kind, Code, SafeMessage string
	Retryable               bool
}
type Report struct {
	ID, GenerationID, OutcomeID, RunID, AssessmentID uint64
	ReportType, TemplateVersion                      string
	GeneratedAt                                      time.Time
}
type Service interface {
	FindReportByID(context.Context, Actor, meta.ID) (*Report, error)
	FindGenerationsByOutcomeID(context.Context, Actor, meta.ID) ([]Generation, error)
	FindLifecycleByAssessmentID(context.Context, Actor, meta.ID) ([]Generation, error)
	ListHistoricalReportsByAssessmentID(context.Context, Actor, meta.ID) ([]Report, error)
}
type service struct {
	outcomes    OutcomeCorrelation
	generations domaingeneration.Repository
	runs        domainrun.Repository
	reports     domainreport.ReportRepository
}

func NewService(outcomes OutcomeCorrelation, g domaingeneration.Repository, r domainrun.Repository, reports domainreport.ReportRepository) Service {
	return &service{outcomes: outcomes, generations: g, runs: r, reports: reports}
}
func (s *service) FindReportByID(ctx context.Context, a Actor, id meta.ID) (*Report, error) {
	if err := s.authorize(a); err != nil {
		return nil, err
	}
	item, err := s.reports.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return mapReport(item), nil
}
func (s *service) FindGenerationsByOutcomeID(ctx context.Context, a Actor, id meta.ID) ([]Generation, error) {
	if err := s.authorize(a); err != nil {
		return nil, err
	}
	if id.IsZero() {
		return nil, fmt.Errorf("evaluation outcome id is required")
	}
	items, err := s.generations.ListByOutcomeID(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.mapGenerations(ctx, items)
}
func (s *service) FindLifecycleByAssessmentID(ctx context.Context, a Actor, id meta.ID) ([]Generation, error) {
	if err := s.authorize(a); err != nil {
		return nil, err
	}
	if id.IsZero() {
		return nil, fmt.Errorf("assessment id is required")
	}
	outcomeID, err := s.outcomes.FindOutcomeIDByAssessmentID(ctx, id)
	if err != nil {
		return nil, err
	}
	items, err := s.generations.ListByOutcomeID(ctx, outcomeID)
	if err != nil {
		return nil, err
	}
	return s.mapGenerations(ctx, items)
}
func (s *service) ListHistoricalReportsByAssessmentID(ctx context.Context, a Actor, id meta.ID) ([]Report, error) {
	if err := s.authorize(a); err != nil {
		return nil, err
	}
	items, err := s.reports.ListByAssessmentID(ctx, id)
	if err != nil {
		return nil, err
	}
	result := make([]Report, 0, len(items))
	for _, item := range items {
		if mapped := mapReport(item); mapped != nil {
			result = append(result, *mapped)
		}
	}
	return result, nil
}
func (s *service) authorize(a Actor) error {
	if s == nil || s.outcomes == nil || s.generations == nil || s.runs == nil || s.reports == nil {
		return cberrors.WithCode(code.ErrModuleInitializationFailed, "interpretation operations service is not configured")
	}
	if a.OperatorUserID == 0 {
		return cberrors.WithCode(code.ErrPermissionDenied, "operations actor is required")
	}
	for _, p := range a.Permissions {
		if p == PermissionAudit {
			return nil
		}
	}
	return cberrors.WithCode(code.ErrPermissionDenied, "interpretation audit permission is required")
}
func (s *service) mapGenerations(ctx context.Context, items []*domaingeneration.ReportGeneration) ([]Generation, error) {
	result := make([]Generation, 0, len(items))
	for _, g := range items {
		if g == nil {
			continue
		}
		key := g.Key()
		item := Generation{ID: g.ID().Uint64(), OutcomeID: key.OutcomeID.Uint64(), ReportType: string(key.ReportType), TemplateVersion: string(key.TemplateVersion), Status: string(g.Status()), LatestRunID: g.LatestRunID().Uint64(), ReportID: g.ReportID().Uint64(), Version: g.Version(), CreatedAt: g.CreatedAt(), UpdatedAt: g.UpdatedAt()}
		if !g.LatestRunID().IsZero() {
			r, err := s.runs.FindByID(ctx, g.LatestRunID())
			if err != nil {
				return nil, err
			}
			item.LatestRun = mapRun(r)
		}
		if g.Status() == domaingeneration.StatusGenerated {
			report, err := s.reports.FindByGenerationID(ctx, g.ID())
			if err != nil {
				return nil, err
			}
			item.Report = mapReport(report)
		}
		result = append(result, item)
	}
	return result, nil
}
func mapRun(r *domainrun.InterpretationRun) *Run {
	if r == nil {
		return nil
	}
	result := &Run{ID: r.ID().Uint64(), GenerationID: r.GenerationID().Uint64(), Attempt: r.Attempt(), Status: string(r.Status()), TraceID: r.TraceID(), StartedAt: r.StartedAt(), LeaseExpiresAt: r.LeaseExpiresAt(), FinishedAt: r.FinishedAt()}
	if f := r.Failure(); f != nil {
		result.Failure = &Failure{Kind: string(f.Kind), Code: f.Code, SafeMessage: f.SafeMessage, Retryable: f.Retryable}
	}
	return result
}
func mapReport(r *domainreport.InterpretReport) *Report {
	if r == nil {
		return nil
	}
	return &Report{ID: r.ID().Uint64(), GenerationID: r.GenerationID().Uint64(), OutcomeID: r.OutcomeID().Uint64(), RunID: r.InterpretationRunID().Uint64(), AssessmentID: r.Association().AssessmentID.Uint64(), ReportType: string(r.ReportType()), TemplateVersion: string(r.TemplateVersion()), GeneratedAt: r.GeneratedAt()}
}
