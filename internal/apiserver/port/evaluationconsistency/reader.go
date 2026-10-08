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
	AssessmentID uint64
	Status       string
	Outcome      *OutcomeEvidence
	Run          *RunEvidence
	Projection   *ProjectionEvidence
	Outbox       *CommittedOutboxEvidence
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
	Class            eventevidence.Class
	InvalidReason    string
	HistoricalReason string
	RowCount         int64
	OutcomeID        string
	RunID            string
	Status           string
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
