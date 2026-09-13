package statistics

import (
	"context"
	"fmt"
	cberrors "github.com/FangcunMount/component-base/pkg/errors"
	authz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// AnalysisCacheRevisionStore fingerprints the current business inputs, separately
// from the immutable published facts. Readers without this port never use cache.
type AnalysisCacheRevisionStore interface {
	AnalysisCacheRevision(context.Context, int64, authz.StoreRange, string) (string, error)
}

// analysisCached keeps permission resolution outside the shared result. Every
// caller has already checked its current Operator, company, action and scope.
func analysisCached[T any](ctx context.Context, s *ReadService, org int64, q *operationQuery, kind string, params any, load func() (T, error)) (T, error) {
	var zero T
	revisions, enabled := s.store.(AnalysisCacheRevisionStore)
	if !enabled || s.cache == nil {
		return load()
	}
	before, err := revisions.AnalysisCacheRevision(ctx, org, q.stores, kind)
	if err != nil {
		return zero, err
	}
	if before == "" {
		return zero, fmt.Errorf("empty statistics business revision")
	}
	key := cacheKey("analysis-v1", kind, q.published.VisibleRunID, q.published.CacheGeneration, q.stores, q.window.From, q.window.To, before, params)
	var value T
	if hit, stale := s.cacheGet(ctx, org, key, &value); hit && !stale {
		return value, nil
	}
	result := s.analysisFlights.DoChan(fmt.Sprintf("%d:%s", org, key), func() (any, error) {
		var cached T
		if hit, stale := s.cacheGet(ctx, org, key, &cached); hit && !stale {
			return cached, nil
		}
		value, err := load()
		if err != nil {
			return zero, err
		}
		after, err := revisions.AnalysisCacheRevision(ctx, org, q.stores, kind)
		if err != nil {
			return zero, err
		}
		if after != before {
			return zero, cberrors.WithCode(code.ErrStatisticsNotReady, "statistics ownership changed; retry query")
		}
		if err = s.checkAnalysisPublication(ctx, org, q); err != nil {
			return zero, err
		}
		s.cacheSet(ctx, org, key, value)
		return value, nil
	})
	select {
	case <-ctx.Done():
		return zero, ctx.Err()
	case out := <-result:
		if out.Err != nil {
			return zero, out.Err
		}
		return out.Val.(T), nil
	}
}
