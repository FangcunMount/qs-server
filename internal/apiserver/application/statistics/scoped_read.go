package statistics

import (
	"context"
	"fmt"
	cberrors "github.com/FangcunMount/component-base/pkg/errors"
	actorctx "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/actorctx"
	authz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	domainstats "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"math"
	"time"
)

// ScopedOverviewData contains components computed under one ownership snapshot.
type ScopedOverviewData struct {
	Metrics OverviewMetrics
	Trends  OverviewTrends
}

// ScopedOverviewStore is separate from the company publication/cache-warm store.
type ScopedOverviewStore interface {
	ScopedOverview(context.Context, int64, authz.StoreRange, time.Time, time.Time, time.Time) (ScopedOverviewData, error)
}

type StatisticsScopeAccess interface {
	ResolveStoreRange(context.Context, int64, int64, string, string) (authz.StoreRange, error)
}

func NewScopedReadService(store ReadStore, access StatisticsScopeAccess, caches ...ReadCache) *ReadService {
	s := NewReadService(store, caches...)
	s.scopeAccess = access
	return s
}
func (s *ReadService) scopedOverview(ctx context.Context, orgID int64, r DateRange, freshness Freshness, permit databaseReadPermit) (*Overview, error) {
	stores, err := s.statisticsRange(ctx, orgID)
	if err != nil {
		return nil, err
	}
	reader, ok := s.store.(ScopedOverviewStore)
	if !ok {
		return nil, fmt.Errorf("scoped statistics reader is not configured")
	}
	if err := ensurePublishedResults(permit.readable); err != nil {
		return nil, err
	}
	asOf, err := time.ParseInLocation("2006-01-02", freshness.AsOfDate, domainstats.Shanghai)
	if err != nil {
		return nil, err
	}
	from, to := queryBounds(r)
	data, err := reader.ScopedOverview(ctx, orgID, stores, from, to, asOf.AddDate(0, 0, 1))
	if err != nil {
		return nil, err
	}
	if err := s.validatePublishedResults(ctx, orgID, permit); err != nil {
		return nil, err
	}
	return buildOverview(orgID, r, freshness, data.Metrics, data.Trends), nil
}

type ScopedClinicianStore interface {
	ScopedClinicians(context.Context, int64, authz.StoreRange, *uint64, *int64, time.Time, time.Time, int, int) ([]ClinicianItem, int64, error)
}

func (s *ReadService) statisticsRange(ctx context.Context, orgID int64) (authz.StoreRange, error) {
	user := actorctx.GrantingUserID(ctx)
	if user == 0 || user > math.MaxInt64 || s.scopeAccess == nil {
		return authz.StoreRange{}, cberrors.WithCode(code.ErrPermissionDenied, "scoped operator access required")
	}
	return s.scopeAccess.ResolveStoreRange(ctx, orgID, int64(user), authz.AssessmentResource, "statistics")
}

type ScopedEntryStore interface {
	ScopedEntries(context.Context, int64, authz.StoreRange, *uint64, *uint64, *bool, time.Time, time.Time, int, int) ([]EntryItem, int64, error)
}

type ScopedContentRef struct {
	ContentRef
	Stores authz.StoreRange
}
type ScopedContentStore interface {
	ScopedContentBatch(context.Context, int64, time.Time, []ScopedContentRef) ([]ContentItem, error)
}

func (s *ReadService) contentRanges(ctx context.Context, orgID int64, refs []ContentRef) ([]ScopedContentRef, error) {
	if len(refs) == 0 || len(refs) > 100 {
		return nil, cberrors.WithCode(code.ErrInvalidArgument, "items must contain 1..100 entries")
	}
	user := actorctx.GrantingUserID(ctx)
	if user == 0 || user > math.MaxInt64 || s.scopeAccess == nil {
		return nil, cberrors.WithCode(code.ErrPermissionDenied, "scoped operator access required")
	}
	seen := map[ContentRef]bool{}
	result := make([]ScopedContentRef, 0, len(refs))
	for _, ref := range refs {
		resource, action := authz.AssessmentModelResource, "read"
		switch ref.Kind {
		case "questionnaire":
			resource, action = authz.QuestionnaireResource, "statistics"
		case "scale", "typology", "behavioral_rating", "cognitive":
		default:
			return nil, cberrors.WithCode(code.ErrInvalidArgument, "unsupported content kind")
		}
		if ref.Code == "" || seen[ref] {
			return nil, cberrors.WithCode(code.ErrInvalidArgument, "empty or duplicate content reference")
		}
		seen[ref] = true
		stores, err := s.scopeAccess.ResolveStoreRange(ctx, orgID, int64(user), resource, action)
		if err != nil {
			return nil, err
		}
		result = append(result, ScopedContentRef{ContentRef: ref, Stores: stores})
	}
	return result, nil
}
