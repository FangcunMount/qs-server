package v2

import (
	"context"
	"fmt"
	"time"
)

var Shanghai = mustShanghai()

func mustShanghai() *time.Location {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		panic(err)
	}
	return location
}

type InstantRange struct{ From, To time.Time }

func (r InstantRange) Validate() error {
	if r.From.IsZero() || r.To.IsZero() || !r.From.Before(r.To) {
		return fmt.Errorf("invalid half-open instant range")
	}
	return nil
}

func BusinessDate(at time.Time) time.Time {
	local := at.In(Shanghai)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, Shanghai)
}

func DefaultWindow(now time.Time, days int) (InstantRange, time.Time) {
	today := BusinessDate(now)
	return InstantRange{From: today.AddDate(0, 0, -days), To: today}, today.AddDate(0, 0, -1)
}

type CollectMode string

const (
	CollectModeNormal   CollectMode = "normal"
	CollectModeValidate CollectMode = "validate_only"
	CollectModeBackfill CollectMode = "backfill"
)

type CollectRequest struct {
	RunID    uint64
	OrgID    int64
	Window   InstantRange
	AsOfDate time.Time
	Mode     CollectMode
}

type CollectResult struct {
	Collector      string
	SourceCount    int64
	InsertedCount  int64
	ExistingCount  int64
	ConflictCount  int64
	FactTypeCounts map[string]int64
}

type FactCollector interface {
	Name() string
	Collect(context.Context, CollectRequest) (CollectResult, error)
}

type CollectorSet struct{ ordered []FactCollector }

func NewCollectorSet(collectors ...FactCollector) (*CollectorSet, error) {
	seen := map[string]struct{}{}
	for _, collector := range collectors {
		if collector == nil || collector.Name() == "" {
			return nil, fmt.Errorf("collector name is required")
		}
		if _, ok := seen[collector.Name()]; ok {
			return nil, fmt.Errorf("duplicate collector %q", collector.Name())
		}
		seen[collector.Name()] = struct{}{}
	}
	return &CollectorSet{ordered: append([]FactCollector(nil), collectors...)}, nil
}

func (s *CollectorSet) Collect(ctx context.Context, request CollectRequest) ([]CollectResult, error) {
	results := make([]CollectResult, 0, len(s.ordered))
	for _, collector := range s.ordered {
		result, err := collector.Collect(ctx, request)
		if err != nil {
			return results, fmt.Errorf("collect %s: %w", collector.Name(), err)
		}
		results = append(results, result)
	}
	return results, nil
}

type ProjectionRequest struct {
	RunID                          uint64
	OrgID                          int64
	Window                         InstantRange
	AsOfDate, CutoffAt, SnapshotAt time.Time
}

type ProjectionResult struct {
	Name string
	Rows int64
}

type Projection interface {
	Name() string
	Project(context.Context, ProjectionRequest) (ProjectionResult, error)
}

type ProjectionEngine struct{ ordered []Projection }

func NewProjectionEngine(projections ...Projection) (*ProjectionEngine, error) {
	seen := map[string]struct{}{}
	for _, projection := range projections {
		if projection == nil || projection.Name() == "" {
			return nil, fmt.Errorf("projection name is required")
		}
		if _, ok := seen[projection.Name()]; ok {
			return nil, fmt.Errorf("duplicate projection %q", projection.Name())
		}
		seen[projection.Name()] = struct{}{}
	}
	return &ProjectionEngine{ordered: append([]Projection(nil), projections...)}, nil
}

func (e *ProjectionEngine) Project(ctx context.Context, request ProjectionRequest) ([]ProjectionResult, error) {
	results := make([]ProjectionResult, 0, len(e.ordered))
	for _, projection := range e.ordered {
		result, err := projection.Project(ctx, request)
		if err != nil {
			return results, fmt.Errorf("project %s: %w", projection.Name(), err)
		}
		results = append(results, result)
	}
	return results, nil
}

type RunStatus string

const (
	RunStatusRunning       RunStatus = "running"
	RunStatusFailed        RunStatus = "failed"
	RunStatusDataCommitted RunStatus = "data_committed"
	RunStatusSucceeded     RunStatus = "succeeded"
)
