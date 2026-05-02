package assessment

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/logger"
)

type myAssessmentListCacheHelper struct {
	cache assessmentListCache
}

type myAssessmentListCacheKey struct {
	userID    uint64
	page      int
	pageSize  int
	status    string
	scaleCode string
	riskLevel string
	dateFrom  string
	dateTo    string
}

func newMyAssessmentListCacheKey(dto ListMyAssessmentsDTO, page, pageSize int) myAssessmentListCacheKey {
	return myAssessmentListCacheKey{
		userID:    dto.TesteeID,
		page:      page,
		pageSize:  pageSize,
		status:    dto.Status,
		scaleCode: dto.ScaleCode,
		riskLevel: dto.RiskLevel,
		dateFrom:  formatAssessmentListDateKey(dto.DateFrom),
		dateTo:    formatAssessmentListDateKey(dto.DateTo),
	}
}

func (h myAssessmentListCacheHelper) Get(ctx context.Context, key myAssessmentListCacheKey) (*AssessmentListResult, bool) {
	if h.cache == nil {
		return nil, false
	}
	var cached AssessmentListResult
	if err := h.cache.Get(
		ctx,
		key.userID,
		key.page,
		key.pageSize,
		key.status,
		key.scaleCode,
		key.riskLevel,
		key.dateFrom,
		key.dateTo,
		&cached,
	); err == nil {
		return &cached, true
	}
	return nil, false
}

func (h myAssessmentListCacheHelper) Set(ctx context.Context, key myAssessmentListCacheKey, result *AssessmentListResult) {
	if h.cache == nil || result == nil {
		return
	}
	h.cache.Set(
		ctx,
		key.userID,
		key.page,
		key.pageSize,
		key.status,
		key.scaleCode,
		key.riskLevel,
		key.dateFrom,
		key.dateTo,
		result,
	)
}

func (h myAssessmentListCacheHelper) Invalidate(ctx context.Context, userID uint64) {
	if h.cache == nil || userID == 0 {
		return
	}

	l := logger.L(ctx)
	startTime := time.Now()

	cacheCtx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	if err := h.cache.Invalidate(cacheCtx, userID); err != nil {
		l.Warnw("失效我的测评列表缓存失败",
			"action", "invalidate_my_assessment_list_cache",
			"user_id", userID,
			"duration_ms", time.Since(startTime).Milliseconds(),
			"error", err.Error(),
		)
		return
	}

	duration := time.Since(startTime)
	if duration > 200*time.Millisecond {
		l.Warnw("失效我的测评列表缓存较慢",
			"action", "invalidate_my_assessment_list_cache",
			"user_id", userID,
			"duration_ms", duration.Milliseconds(),
		)
	}
}

func formatAssessmentListDateKey(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}
