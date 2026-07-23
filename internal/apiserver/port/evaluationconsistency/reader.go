// Package evaluationconsistency defines read-only evidence used by the
// Evaluation consistency scheduler. It intentionally exposes no repair ports.
package evaluationconsistency

import "context"

type ProjectionEvidence struct {
	RowCount             int64
	UnlinkedRowCount     int64
	DistinctOutcomeCount int64
	OutcomeID            string
}

type CommittedOutboxEvidence struct {
	RowCount  int64
	OutcomeID string
	RunID     string
	Status    string
}

type Reader interface {
	FindProjectionEvidence(context.Context, uint64) (*ProjectionEvidence, error)
	FindCommittedOutboxEvidence(context.Context, uint64) (*CommittedOutboxEvidence, error)
}
