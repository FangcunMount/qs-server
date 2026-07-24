package catalogreconcile

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
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
	CountDrifts(context.Context, Filter) (DriftCounts, error)
	ListDrifts(context.Context, Filter, string, int) (DriftPage, error)
	SaveRepairPlan(context.Context, RepairPlan) error
	FindRepairPlan(context.Context, string) (RepairPlan, error)
	RecoverArchiveAssociation(context.Context, uint64, OutcomeAssociation) (string, error)
	ApplyRepair(context.Context, RepairPlan) (string, error)
}

// OutcomeAssociation is the committed Evaluation authority used only to
// recover legacy Archive association metadata. It never carries report
// content into the repair path.
type OutcomeAssociation struct {
	OutcomeID    uint64
	OrgID        int64
	AssessmentID uint64
	TesteeID     uint64
}

type ArchiveAuthority interface {
	FindCommittedOutcome(context.Context, uint64) (OutcomeAssociation, error)
}

// Service runs read-only catalog reconcile. Repair is intentionally separate.
type Service interface {
	ReconcileOnce(context.Context, Filter) (DriftCounts, error)
	ListDrifts(context.Context, Filter, string, int) (DriftPage, error)
	CreateRepairPlan(context.Context, int64, Filter) (RepairPlan, error)
	Repair(context.Context, RepairCommand) (RepairResult, error)
	BindArchiveAuthority(ArchiveAuthority)
}

type AuditService interface {
	ReconcileOnce(context.Context, Filter) (DriftCounts, error)
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
	store     Store
	now       func() time.Time
	newID     func() string
	mu        sync.RWMutex
	authority ArchiveAuthority
}

func NewService(store Store) Service {
	return &service{store: store, now: time.Now, newID: func() string { return meta.New().String() }}
}

func (s *service) BindArchiveAuthority(authority ArchiveAuthority) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.authority = authority
}

func (s *service) archiveAuthority() ArchiveAuthority {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.authority
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
	recoveredArchive := false
	if plan.Item.Source == "archive" && containsField(plan.Item.Fields, "org_id") {
		authority := s.archiveAuthority()
		if authority == nil {
			return RepairResult{}, fmt.Errorf("committed outcome authority is not configured")
		}
		association, err := authority.FindCommittedOutcome(ctx, plan.Item.AssessmentID)
		if err != nil {
			return RepairResult{}, fmt.Errorf("load committed outcome authority: %w", err)
		}
		if association.AssessmentID != plan.Item.AssessmentID || association.OrgID != command.OrgID ||
			association.OutcomeID == 0 || association.TesteeID == 0 {
			return RepairResult{}, fmt.Errorf("committed outcome association does not match repair target")
		}
		status, err := s.store.RecoverArchiveAssociation(ctx, plan.Item.AssessmentID, association)
		if err != nil {
			return RepairResult{}, err
		}
		recoveredArchive = status == "repaired" || status == "already_repaired"
	}
	assessmentID := plan.Item.AssessmentID
	page, err := s.ListDrifts(ctx, Filter{
		OrgID: &command.OrgID, AssessmentID: &assessmentID, Kind: plan.Item.Kind,
	}, "", 2)
	if err != nil {
		return RepairResult{}, err
	}
	if len(page.Items) == 0 {
		status := "already_repaired"
		if recoveredArchive {
			status = "repaired"
		}
		return RepairResult{Status: status, Item: plan.Item}, nil
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

func containsField(fields []string, target string) bool {
	for _, field := range fields {
		if field == target {
			return true
		}
	}
	return false
}

// ScheduledAuditor adapts read-only catalog reconciliation to the shared
// HA consistency scheduler without running on every fast lease-recovery tick.
type ScheduledAuditor struct {
	service     AuditService
	minInterval time.Duration
	mu          sync.Mutex
	lastRun     time.Time
	now         func() time.Time
}

func NewScheduledAuditor(service AuditService, minInterval time.Duration) *ScheduledAuditor {
	if minInterval <= 0 {
		minInterval = 10 * time.Minute
	}
	return &ScheduledAuditor{service: service, minInterval: minInterval, now: time.Now}
}

// AuditOnce implements the Evaluation scheduler consistency-auditor contract.
// The scheduler's limit applies to repairable assessment rows; catalog drift is
// count-only and uses its own fixed Mongo batch size.
func (a *ScheduledAuditor) AuditOnce(ctx context.Context, _ int) (int, error) {
	if a == nil || a.service == nil {
		return 0, fmt.Errorf("catalog reconcile auditor is not configured")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	now := a.now()
	if !a.lastRun.IsZero() && now.Sub(a.lastRun) < a.minInterval {
		return 0, nil
	}
	counts, err := a.service.ReconcileOnce(ctx, Filter{})
	if err != nil {
		return 0, err
	}
	a.lastRun = now
	return int(counts.Total()), nil
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
