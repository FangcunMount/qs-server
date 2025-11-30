package core

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"
	"time"
)

// ==================== 消息处理器自动注册 ====================

// MessageHandlerFactory 消息处理器工厂函数
// 用于延迟创建 MessageHandler，支持依赖注入
type MessageHandlerFactory func(deps *HandlerDependencies) MessageHandler

// HandlerDependencies 处理器依赖
// 包含创建处理器所需的所有依赖
type HandlerDependencies struct {
	Logger *slog.Logger
	// 可扩展其他依赖，如 gRPC 客户端等
	Extra map[string]interface{}
}

// messageHandlerRegistry 消息处理器注册表（内部使用）
var messageHandlerRegistry = struct {
	sync.RWMutex
	// topic -> []MessageHandlerFactory
	factories map[string][]MessageHandlerFactory
}{
	factories: make(map[string][]MessageHandlerFactory),
}

// RegisterMessageHandler 注册消息处理器到指定 Topic
// 在 init() 中调用，实现自动注册
//
// 使用示例:
//
//	func init() {
//	    core.RegisterMessageHandler(core.TopicQuestionnaireLifecycle, func(deps *core.HandlerDependencies) core.MessageHandler {
//	        return NewQuestionnairePublishedHandler(deps.Logger)
//	    })
//	}
func RegisterMessageHandler(topic string, factory MessageHandlerFactory) {
	messageHandlerRegistry.Lock()
	defer messageHandlerRegistry.Unlock()
	messageHandlerRegistry.factories[topic] = append(messageHandlerRegistry.factories[topic], factory)
}

// GetMessageHandlerFactories 获取指定 Topic 的所有处理器工厂
func GetMessageHandlerFactories(topic string) []MessageHandlerFactory {
	messageHandlerRegistry.RLock()
	defer messageHandlerRegistry.RUnlock()
	return messageHandlerRegistry.factories[topic]
}

// ==================== BaseMessageHandler ====================

// BaseMessageHandler 消息处理器基础实现
// 提供事件类型存储，供具体消息处理器嵌入使用
type BaseMessageHandler struct {
	eventType string
}

// NewBaseMessageHandler 创建消息处理器基础实例
func NewBaseMessageHandler(eventType string) *BaseMessageHandler {
	return &BaseMessageHandler{
		eventType: eventType,
	}
}

// EventType 返回事件类型
func (h *BaseMessageHandler) EventType() string {
	return h.eventType
}

// ==================== 指标收集器接口 ====================

// MetricsCollector 指标收集器接口
// 用于收集消息处理的指标数据
type MetricsCollector interface {
	// IncrementEventCount 增加事件计数
	IncrementEventCount(eventType string)
	// RecordEventDuration 记录事件处理耗时
	RecordEventDuration(eventType string, durationMs float64)
	// IncrementEventError 增加事件错误计数
	IncrementEventError(eventType string)
}

// ==================== 钩子接口定义 ====================

// MessageValidator 消息验证器钩子接口
// 实现此接口以提供自定义的消息验证逻辑
type MessageValidator interface {
	// Validate 验证消息载荷
	// 返回 error 表示验证失败，nil 表示验证通过
	Validate(payload []byte) error
}

// MessageParser 消息解析器钩子接口（必须实现）
// 将原始载荷解析为业务对象
type MessageParser interface {
	// Parse 解析消息载荷为业务对象
	Parse(payload []byte) (interface{}, error)
}

// MessageProcessor 消息处理器钩子接口（必须实现）
// 执行业务逻辑处理
type MessageProcessor interface {
	// Process 处理解析后的业务对象
	Process(ctx context.Context, data interface{}) error
}

// BeforeProcessHook 处理前钩子接口（可选）
// 在业务处理之前执行，可用于准备资源、记录日志等
type BeforeProcessHook interface {
	// BeforeProcess 处理前回调
	BeforeProcess(ctx context.Context, payload []byte) error
}

// AfterProcessHook 处理后钩子接口（可选）
// 在业务处理成功后执行，可用于清理资源、发送通知等
type AfterProcessHook interface {
	// AfterProcess 处理后回调
	AfterProcess(ctx context.Context, data interface{}) error
}

// OnErrorHook 错误处理钩子接口（可选）
// 在处理过程中发生错误时调用
type OnErrorHook interface {
	// OnError 错误回调，返回的 error 将作为最终错误
	OnError(ctx context.Context, err error, payload []byte) error
}

// ==================== 模板消息处理器 ====================

// TemplateMessageHandler 模板方法消息处理器
// 定义消息处理的标准流程骨架，子类通过实现钩子接口来定制特定步骤
type TemplateMessageHandler struct {
	*BaseMessageHandler
	logger  *slog.Logger
	metrics MetricsCollector // 可选的指标收集器
}

// NewTemplateMessageHandler 创建模板方法消息处理器
func NewTemplateMessageHandler(eventType string, logger *slog.Logger) *TemplateMessageHandler {
	return &TemplateMessageHandler{
		BaseMessageHandler: NewBaseMessageHandler(eventType),
		logger:             logger,
	}
}

// WithMetrics 配置指标收集器（链式调用）
func (h *TemplateMessageHandler) WithMetrics(collector MetricsCollector) *TemplateMessageHandler {
	h.metrics = collector
	return h
}

// Logger 获取日志记录器
func (h *TemplateMessageHandler) Logger() *slog.Logger {
	return h.logger
}

// Execute 执行模板方法
// 这是真正的模板方法，定义了固定的处理流程骨架
func (h *TemplateMessageHandler) Execute(
	ctx context.Context,
	payload []byte,
	impl interface{},
) (err error) {
	startTime := time.Now()

	// ========== 固定步骤 1: Panic Recovery ==========
	defer func() {
		if r := recover(); r != nil {
			h.logger.Error("panic recovered in message handler",
				slog.String("event_type", h.EventType()),
				slog.Any("panic", r),
				slog.String("stack", string(debug.Stack())),
			)
			// 将 panic 转换为错误
			if e, ok := r.(error); ok {
				err = fmt.Errorf("panic: %w", e)
			} else {
				err = fmt.Errorf("panic: %v", r)
			}
		}

		// ========== 固定步骤: Metrics 记录 ==========
		if h.metrics != nil {
			duration := time.Since(startTime).Seconds() * 1000
			h.metrics.RecordEventDuration(h.EventType(), duration)
			if err != nil {
				h.metrics.IncrementEventError(h.EventType())
			}
		}
	}()

	// ========== 固定步骤: Metrics 计数 ==========
	if h.metrics != nil {
		h.metrics.IncrementEventCount(h.EventType())
	}

	// ========== 固定步骤 2: 前置日志 ==========
	h.logger.Debug("start processing event",
		slog.String("event_type", h.EventType()),
		slog.Int("payload_size", len(payload)),
	)

	// 错误处理包装
	defer func() {
		if err != nil {
			// 调用错误钩子（如果实现）
			if hook, ok := impl.(OnErrorHook); ok {
				err = hook.OnError(ctx, err, payload)
			}
			// 错误日志（固定步骤）
			h.logger.Error("event processing failed",
				slog.String("event_type", h.EventType()),
				slog.String("error", err.Error()),
			)
		}
	}()

	// ========== 可选钩子: BeforeProcess ==========
	if hook, ok := impl.(BeforeProcessHook); ok {
		if err = hook.BeforeProcess(ctx, payload); err != nil {
			return err
		}
	}

	// ========== 可选钩子: Validate ==========
	if validator, ok := impl.(MessageValidator); ok {
		if err = validator.Validate(payload); err != nil {
			return err
		}
		h.logger.Debug("payload validation passed",
			slog.String("event_type", h.EventType()),
		)
	}

	// ========== 必须钩子: Parse ==========
	parser, ok := impl.(MessageParser)
	if !ok {
		panic("impl must implement MessageParser interface")
	}
	var data interface{}
	data, err = parser.Parse(payload)
	if err != nil {
		return err
	}

	// ========== 必须钩子: Process ==========
	processor, ok := impl.(MessageProcessor)
	if !ok {
		panic("impl must implement MessageProcessor interface")
	}
	if err = processor.Process(ctx, data); err != nil {
		return err
	}

	// ========== 可选钩子: AfterProcess ==========
	if hook, ok := impl.(AfterProcessHook); ok {
		if err = hook.AfterProcess(ctx, data); err != nil {
			return err
		}
	}

	// ========== 固定步骤 3: 完成日志 ==========
	h.logger.Debug("event processed successfully",
		slog.String("event_type", h.EventType()),
		slog.String("duration", time.Since(startTime).String()),
	)

	return nil
}
