package evaluation

import (
	"fmt"
	"strings"

	localcache "github.com/FangcunMount/qs-server/internal/pkg/cache/local"
)

const evaluatedAssessmentStatus = "evaluated"

type AssessmentAccessCache interface {
	Has(testeeID, assessmentID uint64) bool
	Set(testeeID, assessmentID uint64)
}

type LocalAssessmentAccessCache struct {
	entries *localcache.Cache[bool]
}

func NewLocalAssessmentAccessCache(opts localcache.Options) *LocalAssessmentAccessCache {
	return &LocalAssessmentAccessCache{entries: localcache.New[bool](opts, nil)}
}

func (c *LocalAssessmentAccessCache) Has(testeeID, assessmentID uint64) bool {
	if c == nil || c.entries == nil || testeeID == 0 || assessmentID == 0 {
		return false
	}
	_, ok := c.entries.Get(assessmentDetailCacheKey(testeeID, assessmentID))
	return ok
}

func (c *LocalAssessmentAccessCache) Set(testeeID, assessmentID uint64) {
	if c == nil || c.entries == nil || testeeID == 0 || assessmentID == 0 {
		return
	}
	c.entries.Set(assessmentDetailCacheKey(testeeID, assessmentID), true)
}

// AssessmentDetailCache is owned by collection's evaluation query boundary.
// It stores only final collection DTOs, never domain aggregates or gRPC DTOs.
type AssessmentDetailCache interface {
	Get(testeeID, assessmentID uint64) (*AssessmentDetailResponse, bool)
	Set(testeeID, assessmentID uint64, value *AssessmentDetailResponse)
}

type LocalAssessmentDetailCache struct {
	entries *localcache.Cache[*AssessmentDetailResponse]
}

func NewLocalAssessmentDetailCache(opts localcache.Options) *LocalAssessmentDetailCache {
	return &LocalAssessmentDetailCache{entries: localcache.New(opts, cloneAssessmentDetailResponse)}
}

func (c *LocalAssessmentDetailCache) Get(testeeID, assessmentID uint64) (*AssessmentDetailResponse, bool) {
	if c == nil || c.entries == nil || testeeID == 0 || assessmentID == 0 {
		return nil, false
	}
	return c.entries.Get(assessmentDetailCacheKey(testeeID, assessmentID))
}

func (c *LocalAssessmentDetailCache) Set(testeeID, assessmentID uint64, value *AssessmentDetailResponse) {
	if c == nil || c.entries == nil || testeeID == 0 || assessmentID == 0 || !isCacheableAssessmentDetail(value) {
		return
	}
	c.entries.Set(assessmentDetailCacheKey(testeeID, assessmentID), value)
}

func assessmentDetailCacheKey(testeeID, assessmentID uint64) string {
	return fmt.Sprintf("%d:%d", testeeID, assessmentID)
}

func isCacheableAssessmentDetail(value *AssessmentDetailResponse) bool {
	return value != nil && strings.EqualFold(strings.TrimSpace(value.Status), evaluatedAssessmentStatus)
}

func cloneAssessmentDetailResponse(value *AssessmentDetailResponse) *AssessmentDetailResponse {
	if value == nil {
		return nil
	}
	cloned := *value
	if value.PrimaryScore != nil {
		primary := *value.PrimaryScore
		if value.PrimaryScore.Max != nil {
			max := *value.PrimaryScore.Max
			primary.Max = &max
		}
		cloned.PrimaryScore = &primary
	}
	if value.Level != nil {
		level := *value.Level
		cloned.Level = &level
	}
	return &cloned
}

var _ AssessmentDetailCache = (*LocalAssessmentDetailCache)(nil)
var _ AssessmentAccessCache = (*LocalAssessmentAccessCache)(nil)
