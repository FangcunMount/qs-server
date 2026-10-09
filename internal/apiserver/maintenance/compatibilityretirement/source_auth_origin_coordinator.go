package retirement

import (
	"context"
	"io"
)

// BindOriginCopies binds reread copies to this coordinator's original full-EOF
// authentication and exact expectations. Supply independently reopened borrowed
// streams so the coordinator's own second-pass cursors remain untouched. This
// does not authenticate production approval or grant evidence-write authority.
func (c *HistoricalCoordinator) BindOriginCopies(ctx context.Context, inputs []SourceCopyInput, limits SourceOriginLimits) (*OriginCopyBinding, error) {
	if c == nil || ctx == nil || len(inputs) != 4 {
		return nil, ErrCoordinatorInvalid
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.alive(ctx); err != nil {
		return nil, err
	}
	for i := range inputs {
		if inputs[i].Expected != c.copies[i].Expected {
			return nil, ErrSourceAuthentication
		}
	}
	binding, err := BindOriginCopies(ctx, c.authenticated, inputs, limits)
	if err != nil {
		return nil, err
	}
	if err = c.alive(ctx); err != nil {
		return nil, err
	}
	return binding, nil
}

// PrepareSourceOriginSnapshotEpoch keeps the raw SQL cycle private and delegates
// to the same actual borrowed-snapshot verifier used by infra-level callers.
func PrepareSourceOriginSnapshotEpoch(ctx context.Context, binding *OriginCopyBinding, sql *SQLResponsibilitySnapshot, mongo *MongoResponsibilitySnapshot, readers []io.Reader) (*SourceOriginEpoch, error) {
	if sql == nil || sql.cycle == nil {
		return nil, ErrSourceOrigin
	}
	return PrepareSourceOriginEpoch(ctx, binding, sql.cycle, mongo, readers)
}

// RecheckSnapshots requires actual independent SQL and Mongo transactions. The
// first epoch may have ended; this method neither opens nor ends either scope.
func (e *SourceOriginEpoch) RecheckSnapshots(ctx context.Context, sql *SQLResponsibilitySnapshot, mongo *MongoResponsibilitySnapshot, readers []io.Reader) (*SourceOriginRecheckProof, error) {
	if sql == nil || sql.cycle == nil {
		return nil, ErrSourceOrigin
	}
	return e.Recheck(ctx, sql.cycle, mongo, readers)
}
