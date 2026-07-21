package catalogreconcile

import (
	"context"
	"fmt"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
)

// DriftKind is one of the four IR-R015 catalog drift classes.
type DriftKind = string

const (
	DriftMissing             DriftKind = "missing"
	DriftDangling            DriftKind = "dangling"
	DriftAssociationMismatch DriftKind = "association_mismatch"
	DriftWrongWinner         DriftKind = "wrong_winner"
)

// Filter scopes read-only reconcile scans.
type Filter struct {
	OrgID        *int64
	SortAtAfter  *time.Time
	SortAtBefore *time.Time
}

// DriftCounts aggregates drift totals for one reconcile pass.
type DriftCounts struct {
	Missing             int64
	Dangling            int64
	AssociationMismatch int64
	WrongWinner         int64
}

func (c DriftCounts) Total() int64 {
	return c.Missing + c.Dangling + c.AssociationMismatch + c.WrongWinner
}

// Store performs read-only catalog drift detection.
type Store interface {
	CountDrifts(context.Context, Filter) (DriftCounts, error)
}

// Service runs read-only catalog reconcile. Repair is intentionally separate.
type Service interface {
	ReconcileOnce(context.Context, Filter) (DriftCounts, error)
}

// RepairAuthorizer must explicitly authorize any mutating repair path.
// IR-R015: repair is not enabled in default deployment.
type RepairAuthorizer interface {
	AuthorizeRepair(context.Context) error
}

type service struct {
	store Store
}

func NewService(store Store) Service {
	return &service{store: store}
}

// ReconcileOnce is dry-run by design: it only counts drift and emits metrics.
func (s *service) ReconcileOnce(ctx context.Context, filter Filter) (DriftCounts, error) {
	if s == nil || s.store == nil {
		return DriftCounts{}, fmt.Errorf("catalog reconcile service is not configured")
	}
	counts, err := s.store.CountDrifts(ctx, filter)
	if err != nil {
		return DriftCounts{}, err
	}
	observeDrift(DriftMissing, counts.Missing)
	observeDrift(DriftDangling, counts.Dangling)
	observeDrift(DriftAssociationMismatch, counts.AssociationMismatch)
	observeDrift(DriftWrongWinner, counts.WrongWinner)
	if counts.Total() > 0 {
		log.Warnf(
			"report catalog drift detected (missing=%d dangling=%d association_mismatch=%d wrong_winner=%d)",
			counts.Missing, counts.Dangling, counts.AssociationMismatch, counts.WrongWinner,
		)
	}
	return counts, nil
}

// Repair is gated behind explicit authorization and deployment switches.
// Do not wire AuthorizeRepair in production until audited runbook exists (IR-R015).
func Repair(ctx context.Context, authorizer RepairAuthorizer, _ Filter) error {
	if authorizer == nil {
		return fmt.Errorf("catalog repair is disabled: repair authorizer is not configured")
	}
	if err := authorizer.AuthorizeRepair(ctx); err != nil {
		return fmt.Errorf("catalog repair denied: %w", err)
	}
	return fmt.Errorf("catalog repair is not implemented")
}
