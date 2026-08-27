// Package source resolves the current immutable standard report and its
// committed EvaluationOutcome for AI explanation input assembly.
package source

import (
	"context"
	"errors"
	"fmt"

	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/interpretationreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

var (
	// ErrNotReady means no current standard report has been published for the
	// assessment yet. It is a participant-visible capability state, not a
	// dependency failure.
	ErrNotReady = errors.New("AI explanation standard report is not ready")
	// ErrNotApplicable means the current report is readable but lacks the
	// provenance required by AIExplanationInput v1 (for example legacy content).
	ErrNotApplicable = errors.New("AI explanation is not applicable to the current standard report")
	// ErrInconsistent means the catalog, report and outcome disagree. Callers
	// must fail closed and must not try another report from the assessment.
	ErrInconsistent = errors.New("AI explanation standard report source is inconsistent")
)

// Current is the only source envelope accepted by the v1 input assembler.
// Both values are immutable read facts and have been association-checked.
type Current struct {
	Report  *domainreport.InterpretReport
	Outcome *evaluationfact.Record
}

type Resolver interface {
	ResolveCurrent(context.Context, meta.ID) (*Current, error)
}

type resolver struct {
	catalog  interpretationreadmodel.BatchReportMetadataReader
	reports  domainreport.ReportRepository
	outcomes evaluationfact.Repository
}

func NewResolver(
	catalog interpretationreadmodel.BatchReportMetadataReader,
	reports domainreport.ReportRepository,
	outcomes evaluationfact.Repository,
) (Resolver, error) {
	if catalog == nil || reports == nil || outcomes == nil {
		return nil, fmt.Errorf("AI explanation current source dependencies are required")
	}
	return &resolver{catalog: catalog, reports: reports, outcomes: outcomes}, nil
}

func (r *resolver) ResolveCurrent(ctx context.Context, assessmentID meta.ID) (*Current, error) {
	if r == nil || r.catalog == nil || r.reports == nil || r.outcomes == nil {
		return nil, fmt.Errorf("AI explanation current source resolver is not configured")
	}
	if assessmentID.IsZero() {
		return nil, fmt.Errorf("AI explanation assessment id is required")
	}
	metadataByAssessment, err := r.catalog.GetCurrentReportMetadataByAssessmentIDs(ctx, []uint64{assessmentID.Uint64()})
	if err != nil {
		return nil, fmt.Errorf("load current standard report catalog metadata: %w", err)
	}
	metadata, ok := metadataByAssessment[assessmentID.Uint64()]
	if !ok || metadata.Status == interpretationreadmodel.CurrentReportMetadataMissing {
		return nil, ErrNotReady
	}
	if metadata.Status != interpretationreadmodel.CurrentReportMetadataFound || metadata.SourceKind != "artifact" || metadata.SourceID == 0 {
		return nil, fmt.Errorf("%w: catalog status=%s source=%s/%d", ErrInconsistent, metadata.Status, metadata.SourceKind, metadata.SourceID)
	}
	reportID := meta.FromUint64(metadata.SourceID)
	report, err := r.reports.FindByID(ctx, reportID)
	if err != nil {
		if errors.Is(err, domainreport.ErrInterpretReportNotFound) {
			return nil, fmt.Errorf("%w: selected report %s is missing", ErrInconsistent, reportID)
		}
		return nil, fmt.Errorf("load current standard report %s: %w", reportID, err)
	}
	if err := validateReport(assessmentID, reportID, report); err != nil {
		return nil, err
	}
	outcome, err := r.outcomes.FindByID(ctx, report.OutcomeID())
	if err != nil {
		if errors.Is(err, evaluationfact.ErrNotFound) {
			return nil, fmt.Errorf("%w: source outcome %s is missing", ErrInconsistent, report.OutcomeID())
		}
		return nil, fmt.Errorf("load current standard report outcome %s: %w", report.OutcomeID(), err)
	}
	if err := validateOutcome(report, outcome); err != nil {
		return nil, err
	}
	return &Current{Report: report, Outcome: outcome}, nil
}

func validateReport(assessmentID, selectedReportID meta.ID, report *domainreport.InterpretReport) error {
	if report == nil || report.ID() != selectedReportID {
		return fmt.Errorf("%w: selected report identity mismatch", ErrInconsistent)
	}
	association := report.Association()
	if association.AssessmentID != assessmentID {
		return fmt.Errorf("%w: selected report assessment mismatch", ErrInconsistent)
	}
	if report.ReportType().String() != "standard" {
		return fmt.Errorf("%w: selected report type %s is unsupported", ErrNotApplicable, report.ReportType())
	}
	if report.BuilderIdentity() == domainreport.UnknownBuilderIdentity || report.ContentSchemaVersion() == domainreport.LegacyContentSchemaVersion {
		return fmt.Errorf("%w: selected report provenance is legacy", ErrNotApplicable)
	}
	if report.BuilderIdentity() == "" || report.ContentSchemaVersion() == "" {
		return fmt.Errorf("%w: selected report provenance is incomplete", ErrInconsistent)
	}
	return nil
}

func validateOutcome(report *domainreport.InterpretReport, outcome *evaluationfact.Record) error {
	if outcome == nil || outcome.ID() != report.OutcomeID() {
		return fmt.Errorf("%w: report outcome identity mismatch", ErrInconsistent)
	}
	association := report.Association()
	if outcome.OrgID() != association.OrgID || outcome.AssessmentID() != association.AssessmentID || outcome.TesteeID() != association.TesteeID {
		return fmt.Errorf("%w: report and outcome association mismatch", ErrInconsistent)
	}
	reportModel := report.Content().Model
	outcomeModel := outcome.Model()
	if reportModel.Kind != string(outcomeModel.Kind) ||
		reportModel.Algorithm != string(outcomeModel.Algorithm) ||
		reportModel.Code != outcomeModel.Code ||
		reportModel.Version != outcomeModel.Version ||
		reportModel.Title != outcomeModel.Title {
		return fmt.Errorf("%w: report and outcome model identity mismatch", ErrInconsistent)
	}
	if outcome.Runtime().DecisionKind == "" {
		return fmt.Errorf("%w: outcome runtime identity is incomplete", ErrNotApplicable)
	}
	return nil
}
