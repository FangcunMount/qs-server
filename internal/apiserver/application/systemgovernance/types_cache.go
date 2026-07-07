package systemgovernance

import (
	"time"

	cachegov "github.com/FangcunMount/qs-server/internal/apiserver/application/cachegovernance"
	"github.com/FangcunMount/qs-server/internal/apiserver/cachemodel"
	"github.com/FangcunMount/qs-server/internal/apiserver/cachetarget"
	"github.com/FangcunMount/qs-server/internal/pkg/cachegovernance/observability"
)

// CacheView 暴露缓存 governance detail。
type CacheView struct {
	GeneratedAt time.Time                 `json:"generated_at"`
	Window      string                    `json:"window"`
	Metrics     MetricsSummary            `json:"metrics"`
	Signals     []Signal                  `json:"signals"`
	Snapshot    *cachegov.StatusSnapshot  `json:"snapshot,omitempty"`
	Components  map[string]ComponentCache `json:"components,omitempty"`
	FamilyRows  []CacheFamilyRow          `json:"family_rows,omitempty"`
	WarmupKinds []CacheWarmupKind         `json:"warmup_kinds,omitempty"`
	Hotsets     []CacheHotsetView         `json:"hotsets,omitempty"`
}

// ComponentCache 保存一个组件 缓存/redis 载荷 使用 fetch 元数据。
type ComponentCache struct {
	Available bool                           `json:"available"`
	Reason    string                         `json:"reason,omitempty"`
	Snapshot  *observability.RuntimeSnapshot `json:"snapshot,omitempty"`
}

// CacheFamilyRow 是面向 UI 缓存 家族 健康度 行 across 组件。
type CacheFamilyRow struct {
	Component           string           `json:"component"`
	Family              string           `json:"family"`
	Profile             string           `json:"profile"`
	Namespace           string           `json:"namespace"`
	AllowWarmup         bool             `json:"allow_warmup"`
	Configured          bool             `json:"configured"`
	Available           bool             `json:"available"`
	Degraded            bool             `json:"degraded"`
	Mode                string           `json:"mode"`
	LastError           string           `json:"last_error,omitempty"`
	LastSuccessAt       time.Time        `json:"last_success_at,omitempty"`
	LastFailureAt       time.Time        `json:"last_failure_at,omitempty"`
	ConsecutiveFailures int              `json:"consecutive_failures"`
	UpdatedAt           time.Time        `json:"updated_at,omitempty"`
	Severity            Severity         `json:"severity"`
	Reason              string           `json:"reason,omitempty"`
	MetricEvidence      []MetricEvidence `json:"metric_evidence,omitempty"`
}

// CacheWarmupKind 描述一个supported manual warmup target 类型。
type CacheWarmupKind struct {
	Kind                 cachetarget.WarmupKind `json:"kind"`
	Family               cachemodel.Family      `json:"family"`
	ScopeExample         string                 `json:"scope_example"`
	SupportsManualWarmup bool                   `json:"supports_manual_warmup"`
}

// CacheHotsetView 暴露recommended manual warmup targets 用于 一个类型。
type CacheHotsetView struct {
	Family         cachemodel.Family      `json:"family,omitempty"`
	Kind           cachetarget.WarmupKind `json:"kind,omitempty"`
	Limit          int64                  `json:"limit,omitempty"`
	Available      bool                   `json:"available"`
	Degraded       bool                   `json:"degraded"`
	Message        string                 `json:"message,omitempty"`
	Items          []CacheHotsetItem      `json:"items"`
	MetricEvidence []MetricEvidence       `json:"metric_evidence,omitempty"`
}

// CacheHotsetItem 是flattened 缓存target.Hotset题目 用于 frontend tables。
type CacheHotsetItem struct {
	Family string                 `json:"family"`
	Kind   cachetarget.WarmupKind `json:"kind"`
	Scope  string                 `json:"scope"`
	Score  float64                `json:"score"`
}
