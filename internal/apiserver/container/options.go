package container

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/cache/subsystem"
	eventsubsystem "github.com/FangcunMount/qs-server/internal/apiserver/eventing/subsystem"
	apiserveroptions "github.com/FangcunMount/qs-server/internal/apiserver/options"
	"github.com/FangcunMount/qs-server/internal/pkg/backpressure"
	genericoptions "github.com/FangcunMount/qs-server/internal/pkg/options"
)

// ContainerOptions 容器配置选项。
type ContainerOptions struct {
	// EventSubsystem 是 resource stage 构造完成的唯一事件运行时。
	EventSubsystem *eventsubsystem.Subsystem
	// Cache 缓存控制选项
	Cache ContainerCacheOptions
	// CacheSubsystem cache 子系统组合根。
	CacheSubsystem *cachebootstrap.Subsystem
	// Backpressure 下游依赖背压 limiter，显式传入各 infra adapter。
	Backpressure BackpressureOptions
	// PlanEntryBaseURL 测评计划任务入口基础地址
	PlanEntryBaseURL string
	// StatisticsRepairWindowDays 统计夜间批处理默认回补窗口
	StatisticsRepairWindowDays int
	// ReportStatus report_status 与 signaling YAML 配置
	ReportStatus *genericoptions.ReportStatusOptions `json:"report_status" mapstructure:"report_status"`
	Signaling    *genericoptions.SignalingOptions    `json:"signaling" mapstructure:"signaling"`
	// Silent suppresses container stdout bootstrap/cleanup prints.
	Silent bool
	// SystemGovernance unified governance facade configuration.
	SystemGovernance *apiserveroptions.SystemGovernanceOptions
}

type BackpressureOptions struct {
	MySQL backpressure.Acquirer
	Mongo backpressure.Acquirer
	IAM   backpressure.Acquirer
}

type ContainerCacheOptions = cachebootstrap.CacheOptions

type ContainerWarmupOptions = cachebootstrap.WarmupOptions

// ContainerCacheFamilyOptions 定义单个缓存 family 的对象级策略。
type ContainerCacheFamilyOptions = cachebootstrap.CacheFamilyOptions
