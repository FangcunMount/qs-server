package container

import (
	"github.com/FangcunMount/component-base/pkg/messaging"

	"github.com/FangcunMount/qs-server/internal/apiserver/cachebootstrap"
	"github.com/FangcunMount/qs-server/internal/pkg/backpressure"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/eventruntime"
)

// ContainerOptions 容器配置选项。
type ContainerOptions struct {
	// MQPublisher 消息队列发布器（可选，传入则启用 MQ 模式）
	MQPublisher messaging.Publisher
	// PublisherMode 事件发布器模式（mq, logging, nop）
	PublisherMode eventruntime.PublishMode
	// EventCatalog 事件契约 catalog，发布器和 outbox topic resolver 共享。
	EventCatalog *eventcatalog.Catalog
	// Env 环境名称（prod, dev, test），用于自动选择发布器模式
	Env string
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
	// Silent suppresses container stdout bootstrap/cleanup prints.
	Silent bool
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

// ContainerCacheTTLOptions 缓存 TTL 配置（0 表示使用默认值）
type ContainerCacheTTLOptions = cachebootstrap.CacheTTLOptions
