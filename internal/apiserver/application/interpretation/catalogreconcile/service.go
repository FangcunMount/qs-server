package catalogreconcile

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/meta"
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
	AssessmentID *uint64
	Kind         DriftKind
	SortAtAfter  *time.Time
	SortAtBefore *time.Time
}

type DriftItem struct {
	CatalogID     string    `json:"catalog_id"`
	ReportID      string    `json:"report_id"`
	AssessmentID  uint64    `json:"assessment_id"`
	Source        string    `json:"source"`
	Kind          DriftKind `json:"kind"`
	Fields        []string  `json:"fields,omitempty"`
	ObservedState string    `json:"observed_state"`
	Version       string    `json:"version"`
}

type DriftPage struct {
	Items      []DriftItem `json:"items"`
	NextCursor string      `json:"next_cursor,omitempty"`
}

type RepairPlan struct {
	DryRunID  string    `json:"dry_run_id"`
	OrgID     int64     `json:"org_id"`
	Item      DriftItem `json:"item"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

type RepairCommand struct {
	OrgID                  int64
	DryRunID               string
	ExpectedCatalogVersion string
	ExpectedSource         string
}

type RepairResult struct {
	Status string    `json:"status"`
	Item   DriftItem `json:"item"`
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
	ListDrifts(context.Context, Filter, string, int) (DriftPage, error)
	SaveRepairPlan(context.Context, RepairPlan) error
	FindRepairPlan(context.Context, string) (RepairPlan, error)
	ApplyRepair(context.Context, RepairPlan) (string, error)
}

// Service runs read-only catalog reconcile. Repair is intentionally separate.
type Service interface {
	LatestAuditSnapshot(context.Context, int64) (AuditSnapshot, error)
	ListDrifts(context.Context, Filter, string, int) (DriftPage, error)
	CreateRepairPlan(context.Context, int64, Filter) (RepairPlan, error)
	Repair(context.Context, RepairCommand) (RepairResult, error)
}

type CatalogService interface {
	Service
	RunnerService
}

func (s *service) ListDrifts(ctx context.Context, filter Filter, cursor string, limit int) (DriftPage, error) {
	if s == nil || s.store == nil {
		return DriftPage{}, fmt.Errorf("catalog reconcile service is not configured")
	}
	if filter.Kind != DriftMissing && filter.Kind != DriftDangling &&
		filter.Kind != DriftAssociationMismatch && filter.Kind != DriftWrongWinner {
		return DriftPage{}, fmt.Errorf("catalog drift kind is required")
	}
	if limit <= 0 || limit > 500 {
		limit = 500
	}
	return s.store.ListDrifts(ctx, filter, cursor, limit)
}

type service struct {
	store             Store
	audit             AuditStore
	auditIndexMu      sync.Mutex
	auditIndexesReady bool
	now               func() time.Time
	newID             func() string
}

func NewService(store Store, audit ...AuditStore) CatalogService {
	var auditStore AuditStore
	if len(audit) > 0 {
		auditStore = audit[0]
	}
	return &service{store: store, audit: auditStore, now: time.Now, newID: func() string { return meta.New().String() }}
}

func (s *service) CreateRepairPlan(ctx context.Context, orgID int64, filter Filter) (RepairPlan, error) {
	if s == nil || s.store == nil || orgID == 0 || filter.AssessmentID == nil || *filter.AssessmentID == 0 {
		return RepairPlan{}, fmt.Errorf("catalog repair dry-run requires org and assessment")
	}
	filter.OrgID = &orgID
	page, err := s.ListDrifts(ctx, filter, "", 2)
	if err != nil {
		return RepairPlan{}, err
	}
	if len(page.Items) != 1 {
		return RepairPlan{}, fmt.Errorf("catalog repair dry-run requires exactly one current drift")
	}
	now := s.now().UTC()
	plan := RepairPlan{
		DryRunID: s.newID(), OrgID: orgID, Item: page.Items[0],
		CreatedAt: now, ExpiresAt: now.Add(7 * 24 * time.Hour),
	}
	if err := s.store.SaveRepairPlan(ctx, plan); err != nil {
		return RepairPlan{}, err
	}
	return plan, nil
}

func (s *service) Repair(ctx context.Context, command RepairCommand) (RepairResult, error) {
	if s == nil || s.store == nil || command.OrgID == 0 || command.DryRunID == "" ||
		command.ExpectedCatalogVersion == "" || command.ExpectedSource == "" {
		return RepairResult{}, fmt.Errorf("catalog repair command is incomplete")
	}
	plan, err := s.store.FindRepairPlan(ctx, command.DryRunID)
	if err != nil {
		return RepairResult{}, err
	}
	if plan.OrgID != command.OrgID || !s.now().Before(plan.ExpiresAt) {
		return RepairResult{}, fmt.Errorf("catalog repair dry-run is unavailable or expired")
	}
	if plan.Item.Version != command.ExpectedCatalogVersion || plan.Item.Source != command.ExpectedSource {
		return RepairResult{}, fmt.Errorf("catalog repair expected state changed")
	}
	assessmentID := plan.Item.AssessmentID
	page, err := s.ListDrifts(ctx, Filter{
		OrgID: &command.OrgID, AssessmentID: &assessmentID, Kind: plan.Item.Kind,
	}, "", 2)
	if err != nil {
		return RepairResult{}, err
	}
	if len(page.Items) == 0 {
		return RepairResult{Status: "already_repaired", Item: plan.Item}, nil
	}
	if len(page.Items) != 1 || page.Items[0].Version != plan.Item.Version || page.Items[0].Source != plan.Item.Source {
		return RepairResult{}, fmt.Errorf("catalog repair candidate changed after dry-run")
	}
	status, err := s.store.ApplyRepair(ctx, plan)
	if err != nil {
		return RepairResult{}, err
	}
	return RepairResult{Status: status, Item: plan.Item}, nil
}
