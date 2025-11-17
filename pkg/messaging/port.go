package messaging

import "context"

// Publisher 消息发布者接口
// 领域层/应用层眼中的发布接口，与具体消息中间件解耦
type Publisher interface {
	// Publish 发布消息到指定主题
	// topic: 主题名称
	// body: 消息体（字节数组）
	Publish(ctx context.Context, topic string, body []byte) error

	// PublishMessage 发布消息对象（支持 Metadata）
	PublishMessage(ctx context.Context, topic string, msg *Message) error

	// Close 关闭发布者，释放资源
	Close() error
}

// Subscriber 消息订阅者接口
// 领域层/应用层眼中的订阅接口
type Subscriber interface {
	// Subscribe 订阅指定主题的消息
	// topic: 主题名称
	// channel: 通道名称（NSQ 中的 channel 概念，用于负载均衡）
	// handler: 消息处理函数
	Subscribe(topic, channel string, handler Handler) error

	// SubscribeWithMiddleware 订阅消息（支持中间件）
	SubscribeWithMiddleware(topic, channel string, handler Handler, middlewares ...Middleware) error

	// Stop 停止订阅
	Stop()

	// Close 关闭订阅者，释放资源
	Close() error
}

// Handler 消息处理函数
// 业务层通过实现此函数来处理接收到的消息
type Handler func(ctx context.Context, msg *Message) error

// Middleware 中间件函数
// 用于在消息处理前后执行额外逻辑（日志、重试、超时、监控等）
type Middleware func(Handler) Handler

// Message 领域消息结构
// 参考 Watermill 设计，支持 UUID、Metadata、Ack/Nack
type Message struct {
	// UUID 全局唯一标识（每条消息都有唯一 ID）
	UUID string

	// Metadata 元数据（用于链路追踪、业务标识等）
	// 例如：trace_id, span_id, user_id, request_id
	Metadata map[string]string

	// Payload 消息负载（实际业务数据）
	Payload []byte

	// ========== 以下字段由 Adapter 填充 ==========

	// Attempts 消息重试次数
	Attempts uint16

	// Timestamp 消息时间戳（纳秒）
	Timestamp int64

	// Topic 消息主题
	Topic string

	// Channel 消息通道
	Channel string

	// ========== 内部字段（确认机制） ==========

	// ack 消息确认函数（由底层 Adapter 注入）
	ack func() error

	// nack 消息拒绝函数（由底层 Adapter 注入）
	nack func() error
}

// NewMessage 创建新消息
func NewMessage(uuid string, payload []byte) *Message {
	return &Message{
		UUID:     uuid,
		Payload:  payload,
		Metadata: make(map[string]string),
	}
}

// Ack 确认消息处理成功
// 调用后，消息不会被重新投递
func (m *Message) Ack() error {
	if m.ack != nil {
		return m.ack()
	}
	return nil
}

// Nack 拒绝消息，触发重试
// 调用后，消息会被重新投递（如果未超过最大重试次数）
func (m *Message) Nack() error {
	if m.nack != nil {
		return m.nack()
	}
	return nil
}

// SetAckFunc 设置确认函数（由 Adapter 调用）
func (m *Message) SetAckFunc(ack func() error) {
	m.ack = ack
}

// SetNackFunc 设置拒绝函数（由 Adapter 调用）
func (m *Message) SetNackFunc(nack func() error) {
	m.nack = nack
}

// Body 向后兼容的别名（返回 Payload）
// Deprecated: 使用 Payload 字段代替
func (m *Message) Body() []byte {
	return m.Payload
}

// ID 向后兼容的别名（返回 UUID）
// Deprecated: 使用 UUID 字段代替
func (m *Message) ID() string {
	return m.UUID
}

// EventBus 事件总线接口
// 组合了发布者和订阅者，提供完整的消息总线功能
type EventBus interface {
	// Publisher 获取发布者
	Publisher() Publisher

	// Subscriber 获取订阅者
	Subscriber() Subscriber

	// Router 获取路由器（用于批量注册处理器）
	Router() *Router

	// Health 健康检查（检查连接状态）
	Health() error

	// Close 关闭事件总线（释放所有资源）
	Close() error
}
