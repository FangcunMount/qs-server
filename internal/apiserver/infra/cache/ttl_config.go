package cache

import (
	"time"

	rediskit "github.com/FangcunMount/component-base/pkg/redis"
)

// TTLOptions 用于全局覆盖默认 TTL
type TTLOptions struct {
	Scale            time.Duration
	Questionnaire    time.Duration
	AssessmentDetail time.Duration
	Testee           time.Duration
	Plan             time.Duration
	Negative         time.Duration
}

// TTLJitterRatio 控制 TTL 抖动，默认 10%（0-1）
var TTLJitterRatio = 0.1

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
	if opts.Testee > 0 {
		DefaultTesteeCacheTTL = opts.Testee
	}
	if opts.Plan > 0 {
		DefaultPlanCacheTTL = opts.Plan
	}
	if opts.Negative > 0 {
		NegativeCacheTTL = opts.Negative
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
	return rediskit.JitterTTL(ttl, TTLJitterRatio)
}
