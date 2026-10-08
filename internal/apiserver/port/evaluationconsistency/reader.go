// Package evaluationconsistency defines read-only evidence used by the
// Evaluation consistency scheduler. It intentionally exposes no repair ports.
package evaluationconsistency

import (
	"context"
	"time"

	eventevidence "github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
)

// OutcomeEvidence is the minimal canonical Outcome identity needed by the
// consistency audit. Large report_input_json and payload_json values are
// deliberately excluded from this read model.
type OutcomeEvidence struct {
	ID        string
	RunID     string
	ModelKind string
}

type RunEvidence struct {
	ID             string
	Status         string
	LeaseExpiresAt *time.Time
}

type AssessmentEvidence struct {
	AssessmentID         uint64
	Status               string
	Outcome              *OutcomeEvidence
	Run                  *RunEvidence
	Projection           *ProjectionEvidence
	Outbox               *CommittedOutboxEvidence
	HistoricalReferences []HistoricalReferenceEvidence
	CommittedHistory     *CommittedHistoricalEvidence
}

// CommittedHistoricalEvidence covers only a physically empty canonical pair.
// It records retained history, with no SDK fingerprint or message row claim.
type CommittedHistoricalEvidence struct {
	Class     eventevidence.Class
	OutcomeID string
	RunID     string
	Reasons   []string
}

// HistoricalReferenceEvidence is a retained source conclusion, separately
// inspected within this business batch. It never means a standard message was
// resolved or that all six retired legacy event types have been covered.
type HistoricalReferenceEvidence struct {
	Owner                 string
	OwnerID               string
	EventID               string
	EventType             string
	Class                 eventevidence.Class
	InvalidReason         string
	RunID                 string
	Attempt               uint
	HistoricalReason      string
	LegacyCanonicalAbsent bool
}

type Batch struct {
	Items         []AssessmentEvidence
	NextCursor    uint64
	CycleComplete bool
}

type ProjectionEvidence struct {
	RowCount             int64
	UnlinkedRowCount     int64
	DistinctOutcomeCount int64
	OutcomeID            string
}

type CommittedOutboxEvidence struct {
	// Derived from physical SQL NULLs and a valid canonical Outcome, never
	// from JSON null, an empty ID, or a caller-supplied absence assertion.
	LegacyCanonicalAbsent bool
	Class                 eventevidence.Class
	InvalidReason         string
	HistoricalReason      string
	RowCount              int64
	OutcomeID             string
	RunID                 string
	Status                string
}

type Reader interface {
	ReadBatch(context.Context, uint64, int) (Batch, error)
}

// CycleReader fixes both directions to bounds captured before a full cycle.
// Delivery state, scheduling timestamps and attempts are never event evidence.
type CycleReader interface {
	BusinessUpperBound(context.Context) (uint64, error)
	ReadBatchTo(context.Context, uint64, uint64, int) (Batch, error)
	OutboxUpperBound(context.Context) (uint64, error)
	ReadOutboxBatch(context.Context, uint64, uint64, int) (ReverseBatch, error)
}
type ReverseConflict struct {
	MessageID    string
	AssessmentID uint64
	Reason       string
}
type ReverseBatch struct {
	Scanned       int
	NextCursor    uint64
	CycleComplete bool
	Conflicts     []ReverseConflict
}
