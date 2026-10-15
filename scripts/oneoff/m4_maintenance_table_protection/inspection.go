package main

import (
	"context"
	"errors"
)

// This selects only the shape to inspect. snapshot still requires this exact
// clean version inside its read-only RR transaction and unchanged observations
// on both sides; a ledger read alone is never a successful inspection.
func readInspectionVersion(ctx context.Context, reader sqlReader) (uint64, error) {
	if ctx.Err() != nil {
		return 0, errors.New("inspection_deadline_exhausted")
	}
	ledger, e := readLedger(ctx, reader)
	if e != nil {
		return 0, e
	}
	row := ledger[0]
	if row.Dirty || (row.Version != 102 && row.Version != 103) {
		return 0, errors.New("inspection_requires_clean102_or103")
	}
	if ctx.Err() != nil {
		return 0, errors.New("inspection_deadline_exhausted")
	}
	return row.Version, nil
}
