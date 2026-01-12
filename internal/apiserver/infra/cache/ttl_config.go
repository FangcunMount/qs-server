package cache

import (
	"math/rand"
	"sync"
	"time"
)

// TTLOptions 用于全局覆盖默认 TTL
type TTLOptions struct {
	Scale            time.Duration
	Questionnaire    time.Duration
	AssessmentDetail time.Duration
	AssessmentStatus time.Duration
	Testee           time.Duration
	Plan             time.Duration
}

// TTLJitterRatio 控制 TTL 抖动，默认 10%（0-1）
var TTLJitterRatio = 0.1

var (
	jitterRand = rand.New(rand.NewSource(time.Now().UnixNano()))
	jitterMu   sync.Mutex
)

// ApplyTTLOptions 覆盖默认 TTL（仅在启动时调用一次）
func ApplyTTLOptions(opts TTLOptions) {
	if opts.Scale > 0 {
		DefaultScaleCacheTTL = opts.Scale
	}
	if opts.Questionnaire > 0 {
		DefaultQuestionnaireCacheTTL = opts.Questionnaire
	}
	if opts.AssessmentDetail > 0 {
		DefaultAssessmentDetailCacheTTL = opts.AssessmentDetail
	}
	if opts.AssessmentStatus > 0 {
		AssessmentStatusCacheTTL = opts.AssessmentStatus
	}
	if opts.Testee > 0 {
		DefaultTesteeCacheTTL = opts.Testee
	}
	if opts.Plan > 0 {
		DefaultPlanCacheTTL = opts.Plan
	}
}

// ApplyTTLJitterRatio 覆盖全局 TTL 抖动比例（0-1）
func ApplyTTLJitterRatio(ratio float64) {
	if ratio < 0 {
		ratio = 0
	}
	if ratio > 1 {
		ratio = 1
	}
	TTLJitterRatio = ratio
}

// JitterTTL 根据全局抖动比例对 TTL 进行抖动，避免同时失效
func JitterTTL(ttl time.Duration) time.Duration {
	if ttl <= 0 || TTLJitterRatio <= 0 {
		return ttl
	}
	maxJitter := time.Duration(float64(ttl) * TTLJitterRatio)
	if maxJitter <= 0 {
		return ttl
	}
	jitterMu.Lock()
	delta := jitterRand.Int63n(int64(maxJitter*2)+1) - int64(maxJitter)
	jitterMu.Unlock()
	result := ttl + time.Duration(delta)
	if result <= 0 {
		return time.Second
	}
	return result
}
