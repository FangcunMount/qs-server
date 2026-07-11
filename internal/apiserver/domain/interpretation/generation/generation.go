package generation

import (
	"fmt"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type ID = meta.ID

// Key is the stable idempotency identity for one report.
// ReportType includes the presentation/audience variant when variants produce
// different reports.
type Key struct {
	OutcomeID       meta.ID
	ReportType      policy.ReportType
	TemplateVersion policy.TemplateVersion
}

func (k Key) Validate() error {
	if k.OutcomeID.IsZero() {
		return fmt.Errorf("evaluation outcome id is required")
	}
	if k.ReportType.IsEmpty() {
		return fmt.Errorf("report type is required")
	}
	if k.TemplateVersion.IsEmpty() {
		return fmt.Errorf("template version is required")
	}
	return nil
}

type Status string

const (
	StatusPending    Status = "pending"
	StatusGenerating Status = "generating"
	StatusGenerated  Status = "generated"
	StatusFailed     Status = "failed"
)

func (s Status) IsValid() bool {
	switch s {
	case StatusPending, StatusGenerating, StatusGenerated, StatusFailed:
		return true
	default:
		return false
	}
}

// ReportGeneration is the aggregate root for a requested report. It tracks
// only intent, current attempt and successful report reference.
type ReportGeneration struct {
	id          ID
	key         Key
	status      Status
	latestRunID meta.ID
	reportID    meta.ID
	version     uint64
	createdAt   time.Time
	updatedAt   time.Time
}

func New(id ID, key Key, at time.Time) (*ReportGeneration, error) {
	if id.IsZero() {
		return nil, fmt.Errorf("report generation id is required")
	}
	if err := key.Validate(); err != nil {
		return nil, err
	}
	if at.IsZero() {
		return nil, fmt.Errorf("report generation created at is required")
	}
	return &ReportGeneration{
		id:        id,
		key:       key,
		status:    StatusPending,
		version:   1,
		createdAt: at,
		updatedAt: at,
	}, nil
}

// Restore rehydrates an aggregate from persistence without weakening the same
// invariants used by the state-transition methods.
func Restore(input RestoreInput) (*ReportGeneration, error) {
	if input.ID.IsZero() {
		return nil, fmt.Errorf("report generation id is required")
	}
	if err := input.Key.Validate(); err != nil {
		return nil, err
	}
	if !input.Status.IsValid() || input.Version == 0 || input.CreatedAt.IsZero() || input.UpdatedAt.IsZero() {
		return nil, fmt.Errorf("report generation persistence state is invalid")
	}
	if input.UpdatedAt.Before(input.CreatedAt) {
		return nil, fmt.Errorf("report generation updated at precedes created at")
	}
	switch input.Status {
	case StatusPending:
		if !input.LatestRunID.IsZero() || !input.ReportID.IsZero() {
			return nil, fmt.Errorf("pending report generation has execution references")
		}
	case StatusGenerating, StatusFailed:
		if input.LatestRunID.IsZero() || !input.ReportID.IsZero() {
			return nil, fmt.Errorf("report generation execution references are invalid")
		}
	case StatusGenerated:
		if input.LatestRunID.IsZero() || input.ReportID.IsZero() {
			return nil, fmt.Errorf("generated report generation references are required")
		}
	}
	return &ReportGeneration{
		id:          input.ID,
		key:         input.Key,
		status:      input.Status,
		latestRunID: input.LatestRunID,
		reportID:    input.ReportID,
		version:     input.Version,
		createdAt:   input.CreatedAt,
		updatedAt:   input.UpdatedAt,
	}, nil
}

type RestoreInput struct {
	ID          ID
	Key         Key
	Status      Status
	LatestRunID meta.ID
	ReportID    meta.ID
	Version     uint64
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Begin associates a newly created InterpretationRun with this Generation.
// A generating or generated Generation must be observed, not started again.
func (g *ReportGeneration) Begin(runID meta.ID, at time.Time) error {
	if g == nil {
		return fmt.Errorf("report generation is required")
	}
	if runID.IsZero() {
		return fmt.Errorf("interpretation run id is required")
	}
	if at.IsZero() {
		return fmt.Errorf("report generation started at is required")
	}
	if g.status != StatusPending && g.status != StatusFailed {
		return fmt.Errorf("report generation cannot begin from status %s", g.status)
	}
	g.status = StatusGenerating
	g.latestRunID = runID
	g.updatedAt = at
	g.version++
	return nil
}

func (g *ReportGeneration) Succeed(runID, reportID meta.ID, at time.Time) error {
	if g == nil {
		return fmt.Errorf("report generation is required")
	}
	if runID.IsZero() || reportID.IsZero() {
		return fmt.Errorf("interpretation run id and report id are required")
	}
	if at.IsZero() {
		return fmt.Errorf("report generation completed at is required")
	}
	if g.status != StatusGenerating || g.latestRunID != runID {
		return fmt.Errorf("report generation cannot succeed for run %s from status %s", runID, g.status)
	}
	g.status = StatusGenerated
	g.reportID = reportID
	g.updatedAt = at
	g.version++
	return nil
}

func (g *ReportGeneration) Fail(runID meta.ID, at time.Time) error {
	if g == nil {
		return fmt.Errorf("report generation is required")
	}
	if runID.IsZero() {
		return fmt.Errorf("interpretation run id is required")
	}
	if at.IsZero() {
		return fmt.Errorf("report generation failed at is required")
	}
	if g.status != StatusGenerating || g.latestRunID != runID {
		return fmt.Errorf("report generation cannot fail for run %s from status %s", runID, g.status)
	}
	g.status = StatusFailed
	g.updatedAt = at
	g.version++
	return nil
}

func (g *ReportGeneration) ID() ID { return g.id }

func (g *ReportGeneration) Key() Key { return g.key }

func (g *ReportGeneration) Status() Status { return g.status }

func (g *ReportGeneration) LatestRunID() meta.ID { return g.latestRunID }

func (g *ReportGeneration) ReportID() meta.ID { return g.reportID }

func (g *ReportGeneration) Version() uint64 { return g.version }

func (g *ReportGeneration) CreatedAt() time.Time { return g.createdAt }

func (g *ReportGeneration) UpdatedAt() time.Time { return g.updatedAt }
