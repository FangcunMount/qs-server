// Package operations exposes Interpretation lifecycle diagnostics to
// explicitly authorized operations and audit actors.
package operations

import (
	"context"
	"fmt"
	"time"

	cberrors "github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/admission"
	domaingeneration "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/generation"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	domainrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/run"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
)

type Actor struct {
	OrgID, OperatorUserID int64
}
type OutcomeRef struct {
	ID, AssessmentID meta.ID
	OrgID            int64
}
type OutcomeCorrelation interface {
	FindOutcomeByID(context.Context, meta.ID) (OutcomeRef, error)
	FindOutcomeByAssessmentID(context.Context, meta.ID) (OutcomeRef, error)
}
type Access interface {
	AuthorizeAudit(context.Context, Actor, int64) error
}
type ArtifactMetadata struct {
	ID, AssessmentID, GenerationID meta.ID
	OrgID                          int64
}
type ArtifactMetadataReader interface {
	FindMetadataByID(context.Context, meta.ID) (*ArtifactMetadata, error)
}
type Generation struct {
	ID, OutcomeID, LatestRunID, ReportID uint64
	ReportType, TemplateVersion, Status  string
	Version                              uint64
	CreatedAt, UpdatedAt                 time.Time
	LatestRun                            *Run
	Runs                                 []Run
	Report                               *Report
}
type Run struct {
	ID, GenerationID                                 uint64
	Attempt                                          int
	Status, TraceID                                  string
	Failure                                          *Failure
	StartedAt, LeaseExpiresAt, FinishedAt            *time.Time
	AttemptOrigin, RetryDisposition                  string
	GovernanceStatus                                 string
	MaxAutomaticAttempts, RemainingAutomaticAttempts int
	NextAttemptAt                                    *time.Time
	RetryEventID, ActionRequestID                    string
	RecoveryCount                                    int
	LastReclaimedAt                                  *time.Time
	ClaimHistory                                     []ClaimHistoryEntry
}
type ClaimHistoryEntry struct {
	ReclaimedAt time.Time
	TraceID     string
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
type AdmissionFailure struct {
	ID, OutcomeID, AssessmentID uint64
	OrgID                       int64
	TesteeID                    uint64
	EventID, TraceID            string
	Kind, Code, SafeMessage     string
	Retryable                   bool
	Fingerprint                 string
	OccurredAt                  time.Time
}

type Service interface {
	FindReportByID(context.Context, Actor, meta.ID) (*Report, error)
	FindGenerationsByOutcomeID(context.Context, Actor, meta.ID) ([]Generation, error)
	FindLifecycleByAssessmentID(context.Context, Actor, meta.ID) ([]Generation, error)
	ListHistoricalReportsByAssessmentID(context.Context, Actor, meta.ID) ([]Report, error)
	FindAdmissionFailuresByOutcomeID(context.Context, Actor, meta.ID) ([]AdmissionFailure, error)
}
type service struct {
	outcomes    OutcomeCorrelation
	generations domaingeneration.Repository
	runs        domainrun.Repository
	reports     domainreport.ReportRepository
	metadata    ArtifactMetadataReader
	admissions  admission.Repository
	access      Access
}

func NewService(outcomes OutcomeCorrelation, g domaingeneration.Repository, r domainrun.Repository, reports domainreport.ReportRepository, access Access, admissions ...admission.Repository) Service {
	var admissionRepo admission.Repository
	if len(admissions) > 0 {
		admissionRepo = admissions[0]
	}
	metadata, _ := reports.(ArtifactMetadataReader)
	return &service{outcomes: outcomes, generations: g, runs: r, reports: reports, metadata: metadata, access: access, admissions: admissionRepo}
}
func (s *service) FindReportByID(ctx context.Context, a Actor, id meta.ID) (*Report, error) {
	if err := s.ensureConfigured(); err != nil {
		return nil, err
	}
	if id.IsZero() {
		return nil, fmt.Errorf("interpretation report id is required")
	}
	metadata, err := s.metadata.FindMetadataByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if metadata == nil {
		return nil, domainreport.ErrInterpretReportNotFound
	}
	if err := s.authorize(ctx, a, metadata.OrgID); err != nil {
		return nil, err
	}
	item, err := s.reports.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return mapReport(item), nil
}
func (s *service) FindGenerationsByOutcomeID(ctx context.Context, a Actor, id meta.ID) ([]Generation, error) {
	if err := s.ensureConfigured(); err != nil {
		return nil, err
	}
	if id.IsZero() {
		return nil, fmt.Errorf("evaluation outcome id is required")
	}
	ref, err := s.outcomes.FindOutcomeByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.authorize(ctx, a, ref.OrgID); err != nil {
		return nil, err
	}
	items, err := s.generations.ListByOutcomeID(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.mapGenerations(ctx, items)
}
func (s *service) FindLifecycleByAssessmentID(ctx context.Context, a Actor, id meta.ID) ([]Generation, error) {
	if err := s.ensureConfigured(); err != nil {
		return nil, err
	}
	if id.IsZero() {
		return nil, fmt.Errorf("assessment id is required")
	}
	ref, err := s.outcomes.FindOutcomeByAssessmentID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.authorize(ctx, a, ref.OrgID); err != nil {
		return nil, err
	}
	items, err := s.generations.ListByOutcomeID(ctx, ref.ID)
	if err != nil {
		return nil, err
	}
	return s.mapGenerations(ctx, items)
}
func (s *service) ListHistoricalReportsByAssessmentID(ctx context.Context, a Actor, id meta.ID) ([]Report, error) {
	if err := s.ensureConfigured(); err != nil {
		return nil, err
	}
	ref, err := s.outcomes.FindOutcomeByAssessmentID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.authorize(ctx, a, ref.OrgID); err != nil {
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

func (s *service) FindAdmissionFailuresByOutcomeID(ctx context.Context, a Actor, id meta.ID) ([]AdmissionFailure, error) {
	if err := s.ensureConfigured(); err != nil {
		return nil, err
	}
	if s.admissions == nil {
		return nil, cberrors.WithCode(code.ErrModuleInitializationFailed, "interpretation admission repository is not configured")
	}
	if id.IsZero() {
		return nil, fmt.Errorf("evaluation outcome id is required")
	}
	ref, err := s.outcomes.FindOutcomeByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.authorize(ctx, a, ref.OrgID); err != nil {
		return nil, err
	}
	items, err := s.admissions.FindByOutcomeID(ctx, id, 50)
	if err != nil {
		return nil, err
	}
	result := make([]AdmissionFailure, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		result = append(result, AdmissionFailure{
			ID: item.ID().Uint64(), OutcomeID: item.OutcomeID().Uint64(), AssessmentID: item.AssessmentID().Uint64(),
			OrgID: item.OrgID(), TesteeID: item.TesteeID(), EventID: item.EventID(), TraceID: item.TraceID(),
			Kind: string(item.Kind()), Code: item.Code(), SafeMessage: item.SafeMessage(), Retryable: item.Retryable(),
			Fingerprint: item.Fingerprint(), OccurredAt: item.OccurredAt(),
		})
	}
	return result, nil
}

func (s *service) authorize(ctx context.Context, a Actor, resourceOrgID int64) error {
	if a.OperatorUserID == 0 || a.OrgID == 0 || resourceOrgID == 0 {
		return cberrors.WithCode(code.ErrPermissionDenied, "operations actor is required")
	}
	return s.access.AuthorizeAudit(ctx, a, resourceOrgID)
}

func (s *service) ensureConfigured() error {
	if s == nil || s.outcomes == nil || s.generations == nil || s.runs == nil || s.reports == nil || s.metadata == nil || s.access == nil {
		return cberrors.WithCode(code.ErrModuleInitializationFailed, "interpretation operations service is not configured")
	}
	return nil
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
			if history, ok := s.runs.(domainrun.HistoryReader); ok {
				runs, err := history.ListByGenerationID(ctx, g.ID(), 100)
				if err != nil {
					return nil, err
				}
				item.Runs = make([]Run, 0, len(runs))
				for _, runRecord := range runs {
					if mapped := mapRun(runRecord); mapped != nil {
						item.Runs = append(item.Runs, *mapped)
					}
				}
			} else if item.LatestRun != nil {
				item.Runs = []Run{*item.LatestRun}
			}
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
	result.AttemptOrigin = string(r.Origin())
	result.RecoveryCount = r.RecoveryCount()
	result.LastReclaimedAt = r.LastReclaimedAt()
	if history := r.ClaimHistory(); len(history) > 0 {
		result.ClaimHistory = make([]ClaimHistoryEntry, len(history))
		for i, record := range history {
			result.ClaimHistory[i] = ClaimHistoryEntry{ReclaimedAt: record.ReclaimedAt, TraceID: record.TraceID}
		}
	}
	if decision := r.RetryDecision(); decision != nil {
		result.RetryDisposition = string(decision.Disposition)
		result.GovernanceStatus = governanceStatusForDisposition(decision.Disposition)
		result.MaxAutomaticAttempts = decision.MaxAutomaticAttempts
		result.RemainingAutomaticAttempts = decision.RemainingAutomaticAttempts
		result.NextAttemptAt = decision.NextAttemptAt
		result.RetryEventID = decision.RetryEventID
		result.ActionRequestID = decision.ActionRequestID
	}
	if f := r.Failure(); f != nil {
		result.Failure = &Failure{Kind: string(f.Kind), Code: f.Code, SafeMessage: f.SafeMessage, Retryable: f.Retryable}
	}
	return result
}

func governanceStatusForDisposition(disposition retrygovernance.Disposition) string {
	switch disposition {
	case retrygovernance.DispositionManualRequired:
		return "waiting_manual_action"
	case retrygovernance.DispositionAutomatic:
		return "waiting_automatic_retry"
	case retrygovernance.DispositionTerminal:
		return "terminal"
	default:
		return ""
	}
}
func mapReport(r *domainreport.InterpretReport) *Report {
	if r == nil {
		return nil
	}
	return &Report{ID: r.ID().Uint64(), GenerationID: r.GenerationID().Uint64(), OutcomeID: r.OutcomeID().Uint64(), RunID: r.InterpretationRunID().Uint64(), AssessmentID: r.Association().AssessmentID.Uint64(), ReportType: string(r.ReportType()), TemplateVersion: string(r.TemplateVersion()), GeneratedAt: r.GeneratedAt()}
}
