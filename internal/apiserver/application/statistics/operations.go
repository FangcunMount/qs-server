package statistics

import (
	"context"
	cberrors "github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/actor/actorctx"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"math"
	"sort"
	"time"
)

const OperationsResource = "qs:statistics:collection:operations"

type OperationsFilter struct {
	From, To string
	StoreIDs []uint64
}
type ActivityCounts struct {
	Submissions int64 `json:"submissions"`
	Completions int64 `json:"completions"`
}
type OperationStore struct {
	ID                  uint64 `json:"id,string"`
	Code                string `json:"code"`
	Name                string `json:"name"`
	IsActive            bool   `json:"is_active"`
	CurrentServiceCount int64  `json:"current_service_count"`
	ActivityCounts
}
type ActivityDay struct {
	Date string `json:"date"`
	ActivityCounts
}
type ActivityRow struct {
	StoreID                  uint64
	UnknownReason            string
	Date                     time.Time
	Submissions, Completions int64
}
type OperationsOverview struct {
	Scope                   string    `json:"scope"`
	From                    string    `json:"from"`
	ToExclusive             string    `json:"to_exclusive"`
	WorkloadThrough         string    `json:"workload_through"`
	PublishedAt             time.Time `json:"published_at"`
	PublishedVersion        uint64    `json:"published_version,string"`
	CurrentPopulationReadAt time.Time `json:"current_population_read_at"`
	CurrentServiceCount     int64     `json:"current_service_count"`
	ActivityCounts
	Unknown        *ActivityCounts           `json:"unknown,omitempty"`
	UnknownReasons map[string]ActivityCounts `json:"unknown_reasons,omitempty"`
	Stores         []OperationStore          `json:"stores"`
	Daily          []ActivityDay             `json:"daily"`
}

// OperationsStore keeps pure counts separate from professional result DTOs.
type OperationsStore interface {
	OperationsCoverage(context.Context, int64, uint64, time.Time, time.Time) ([]domain.InstantRange, error)
	OperationsPopulation(context.Context, int64, authz.StoreRange) ([]OperationStore, error)
	OperationsActivity(context.Context, int64, authz.StoreRange, time.Time, time.Time) ([]ActivityRow, error)
}

func operationRange(now time.Time, filter OperationsFilter) (domain.InstantRange, error) {
	today := domain.BusinessDate(now)
	from := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, domain.Shanghai)
	to := today.AddDate(0, 0, 1)
	if filter.From != "" || filter.To != "" {
		var err error
		from, err = time.ParseInLocation("2006-01-02", filter.From, domain.Shanghai)
		if err != nil {
			return domain.InstantRange{}, cberrors.WithCode(code.ErrInvalidArgument, "invalid from date")
		}
		to, err = time.ParseInLocation("2006-01-02", filter.To, domain.Shanghai)
		if err != nil {
			return domain.InstantRange{}, cberrors.WithCode(code.ErrInvalidArgument, "invalid exclusive to date")
		}
	}
	if !from.Before(to) || to.After(today.AddDate(0, 0, 1)) || to.Sub(from) > 366*24*time.Hour {
		return domain.InstantRange{}, cberrors.WithCode(code.ErrInvalidArgument, "invalid operations date range")
	}
	return domain.InstantRange{From: from, To: to}, nil
}
func requestedOperationScope(allowed authz.StoreRange, ids []uint64) (authz.StoreRange, error) {
	if len(ids) == 0 {
		return allowed, nil
	}
	if len(ids) > 500 {
		return authz.StoreRange{}, cberrors.WithCode(code.ErrInvalidArgument, "too many requested stores")
	}
	result := authz.StoreRange{}
	seen := map[uint64]bool{}
	for _, id := range ids {
		if id == 0 || id > math.MaxInt64 || seen[id] {
			return result, cberrors.WithCode(code.ErrInvalidArgument, "invalid or duplicate store")
		}
		if !allowed.Contains(&id) {
			return result, cberrors.WithCode(code.ErrPermissionDenied, "store outside granted range")
		}
		seen[id] = true
		result.StoreIDs = append(result.StoreIDs, id)
	}
	sort.Slice(result.StoreIDs, func(i, j int) bool { return result.StoreIDs[i] < result.StoreIDs[j] })
	return result, nil
}
func coversOperationWindow(windows []domain.InstantRange, requested domain.InstantRange) bool {
	sort.Slice(windows, func(i, j int) bool { return windows[i].From.Before(windows[j].From) })
	cursor := requested.From
	for _, window := range windows {
		if window.From.After(cursor) {
			return false
		}
		if window.To.After(cursor) {
			cursor = window.To
		}
		if !cursor.Before(requested.To) {
			return true
		}
	}
	return false
}
func (s *ReadService) Operations(ctx context.Context, org int64, filter OperationsFilter) (*OperationsOverview, error) {
	if err := authz.RequirePermission(ctx, OperationsResource, "read"); err != nil {
		return nil, err
	}
	user := actorctx.GrantingUserID(ctx)
	if user == 0 || user > math.MaxInt64 || s.scopeAccess == nil {
		return nil, cberrors.WithCode(code.ErrPermissionDenied, "active scoped operator required")
	}
	allowed, err := s.scopeAccess.ResolveStoreRange(ctx, org, int64(user), OperationsResource, "read")
	if err != nil {
		return nil, err
	}
	stores, err := requestedOperationScope(allowed, filter.StoreIDs)
	if err != nil {
		return nil, err
	}
	requested, err := operationRange(s.now(), filter)
	if err != nil {
		return nil, err
	}
	reader, ok := s.store.(OperationsStore)
	if !ok {
		return nil, cberrors.WithCode(code.ErrStatisticsNotReady, "operations reader unavailable")
	}
	published, err := s.store.LatestVisibleSnapshot(ctx, org)
	if err != nil {
		return nil, err
	}
	if published == nil || !published.DatabaseReadable {
		return nil, cberrors.WithCode(code.ErrStatisticsNotReady, "operations not published")
	}
	cutoff := domain.BusinessDate(published.AsOfDate).AddDate(0, 0, 1)
	window := requested
	if window.To.After(cutoff) {
		window.To = cutoff
	}
	if !window.From.Before(window.To) {
		return nil, cberrors.WithCode(code.ErrStatisticsNotReady, "requested period not published")
	}
	coverage, err := reader.OperationsCoverage(ctx, org, published.VisibleRunID, window.From, window.To)
	if err != nil {
		return nil, err
	}
	if !coversOperationWindow(coverage, window) {
		return nil, cberrors.WithCode(code.ErrStatisticsNotReady, "operations period not fully rebuilt")
	}
	population, err := reader.OperationsPopulation(ctx, org, stores)
	if err != nil {
		return nil, err
	}
	var activity []ActivityRow
	key := cacheKey("operations-v1", published.VisibleRunID, stores, window.From, window.To)
	hit, _ := s.cacheGet(ctx, org, key, &activity)
	if !hit {
		activity, err = reader.OperationsActivity(ctx, org, stores, window.From, window.To)
		if err != nil {
			return nil, err
		}
	}
	if err := s.validatePublishedResults(ctx, org, databaseReadPermit{visibleRunID: published.VisibleRunID, readable: true}); err != nil {
		return nil, err
	}
	if !hit {
		s.cacheSet(ctx, org, key, activity)
	}
	return buildOperations(stores, requested, window, published, population, activity, s.now()), nil
}
func buildOperations(scope authz.StoreRange, requested, window domain.InstantRange, published *Snapshot, stores []OperationStore, activity []ActivityRow, readAt time.Time) *OperationsOverview {
	result := &OperationsOverview{Scope: "stores", From: requested.From.Format("2006-01-02"), ToExclusive: requested.To.Format("2006-01-02"), WorkloadThrough: window.To.AddDate(0, 0, -1).Format("2006-01-02"), PublishedAt: published.SnapshotAt, PublishedVersion: published.VisibleRunID, CurrentPopulationReadAt: readAt, Stores: stores, Daily: []ActivityDay{}}
	if scope.AllStores {
		result.Scope = "all_stores"
		result.Unknown = &ActivityCounts{}
		result.UnknownReasons = map[string]ActivityCounts{}
	}
	storeIndex := map[uint64]int{}
	for i, store := range result.Stores {
		storeIndex[store.ID] = i
		result.CurrentServiceCount += store.CurrentServiceCount
	}
	dates := map[string]ActivityCounts{}
	for _, row := range activity {
		result.Submissions += row.Submissions
		result.Completions += row.Completions
		date := domain.BusinessDate(row.Date).Format("2006-01-02")
		counts := dates[date]
		counts.Submissions += row.Submissions
		counts.Completions += row.Completions
		dates[date] = counts
		if row.StoreID == 0 && result.Unknown != nil {
			result.Unknown.Submissions += row.Submissions
			result.Unknown.Completions += row.Completions
			v := result.UnknownReasons[row.UnknownReason]
			v.Submissions += row.Submissions
			v.Completions += row.Completions
			result.UnknownReasons[row.UnknownReason] = v
		}
		if index, ok := storeIndex[row.StoreID]; ok {
			result.Stores[index].Submissions += row.Submissions
			result.Stores[index].Completions += row.Completions
		}
	}
	for date := window.From; date.Before(window.To); date = date.AddDate(0, 0, 1) {
		key := date.Format("2006-01-02")
		result.Daily = append(result.Daily, ActivityDay{Date: key, ActivityCounts: dates[key]})
	}
	return result
}
