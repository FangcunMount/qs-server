package event

import "context"

// EventPublisher 领域事件发布端口
// 这是领域层定义的接口，由基础设施层提供具体实现
//
// 设计原则（六边形架构）：
// - 领域层定义端口（Port）
// - 基础设施层提供适配器（Adapter）
// - 领域层不知道消息队列的存在
//
// 实现示例：
// - MessagingEventPublisher: 通过 pkg/messaging 发布到 NSQ/RabbitMQ
// - InMemoryEventPublisher: 内存实现，用于测试
// - LoggingEventPublisher: 仅记录日志，用于开发调试
type EventPublisher interface {
	// Publish 发布单个领域事件
	//
	// 参数：
	//   - ctx: 上下文，用于超时控制和链路追踪
	//   - event: 领域事件
	//
	// 返回：
	//   - error: 发布失败时返回错误
	Publish(ctx context.Context, event DomainEvent) error

	// PublishAll 批量发布领域事件
	//
	// 参数：
	//   - ctx: 上下文
	//   - events: 领域事件列表
	//
	// 返回：
	//   - error: 任一事件发布失败时返回错误
	PublishAll(ctx context.Context, events []DomainEvent) error
}

// EventSubscriber 领域事件订阅端口
// 用于订阅和处理领域事件
type EventSubscriber interface {
	// Subscribe 订阅指定类型的事件
	//
	// 参数：
	//   - eventType: 事件类型，如 "assessment.submitted"
	//   - handler: 事件处理函数
	Subscribe(eventType string, handler EventHandler) error

	// Start 启动订阅者
	Start(ctx context.Context) error

	// Stop 停止订阅者
	Stop() error
}

// EventHandler 事件处理函数类型
type EventHandler func(ctx context.Context, event DomainEvent) error

// ==================== 辅助接口 ====================

// EventStore 事件存储端口（用于事件溯源场景）
// 当前版本为可选功能，后续可扩展支持
type EventStore interface {
	// Save 保存事件
	Save(ctx context.Context, events []DomainEvent) error

	// Load 加载聚合根的所有事件
	Load(ctx context.Context, aggregateType, aggregateID string) ([]DomainEvent, error)

	// LoadFrom 从指定版本加载事件
	LoadFrom(ctx context.Context, aggregateType, aggregateID string, fromVersion int64) ([]DomainEvent, error)
}

// ==================== 空实现（用于测试和占位） ====================

// NopEventPublisher 空实现的事件发布者
// 用于测试或不需要事件发布的场景
type NopEventPublisher struct{}

// NewNopEventPublisher 创建空实现
func NewNopEventPublisher() *NopEventPublisher {
	return &NopEventPublisher{}
}

// Publish 空实现
func (p *NopEventPublisher) Publish(ctx context.Context, event DomainEvent) error {
	return nil
}

// PublishAll 空实现
func (p *NopEventPublisher) PublishAll(ctx context.Context, events []DomainEvent) error {
	return nil
}
