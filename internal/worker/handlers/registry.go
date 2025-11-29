package handlers

import (
	"log/slog"

	"github.com/FangcunMount/qs-server/internal/worker/infra/grpcclient"
)

// Registry 处理器注册表
type Registry struct {
	handlers map[string]Handler
	logger   *slog.Logger
}

// NewRegistry 创建处理器注册表
func NewRegistry(logger *slog.Logger) *Registry {
	return &Registry{
		handlers: make(map[string]Handler),
		logger:   logger,
	}
}

// Register 注册处理器
func (r *Registry) Register(handler Handler) {
	r.handlers[handler.Topic()] = handler
	r.logger.Info("handler registered",
		slog.String("topic", handler.Topic()),
		slog.String("handler", handler.Name()),
	)
}

// Get 获取处理器
func (r *Registry) Get(topic string) (Handler, bool) {
	handler, ok := r.handlers[topic]
	return handler, ok
}

// Topics 获取所有已注册的主题
func (r *Registry) Topics() []string {
	topics := make([]string, 0, len(r.handlers))
	for topic := range r.handlers {
		topics = append(topics, topic)
	}
	return topics
}

// All 获取所有处理器
func (r *Registry) All() []Handler {
	handlers := make([]Handler, 0, len(r.handlers))
	for _, handler := range r.handlers {
		handlers = append(handlers, handler)
	}
	return handlers
}

// HandlerDeps 处理器依赖
type HandlerDeps struct {
	Logger            *slog.Logger
	AnswerSheetClient *grpcclient.AnswerSheetClient
	EvaluationClient  *grpcclient.EvaluationClient
}

// RegisterDefaultHandlers 注册默认处理器
func RegisterDefaultHandlers(registry *Registry, deps *HandlerDeps) {
	// ==================== 答卷/测评生命周期事件 ====================

	// 1. 答卷保存事件处理器（来自 collection-server）
	// 职责：判断是否需要创建 Assessment
	registry.Register(NewAnswerSheetSavedHandler(deps.Logger))

	// 2. 测评提交事件处理器（来自 apiserver）
	// 职责：执行评估计算
	registry.Register(NewAssessmentSubmittedHandler(deps.Logger, deps.AnswerSheetClient))

	// 3. 测评解读完成事件处理器（来自 apiserver）
	// 职责：发送通知、预警、更新统计
	registry.Register(NewAssessmentInterpretedHandler(deps.Logger))

	// 4. 测评失败事件处理器（来自 apiserver）
	// 职责：记录失败、监控告警
	registry.Register(NewAssessmentFailedHandler(deps.Logger))

	// ==================== 问卷生命周期事件 ====================

	// 5. 问卷发布事件处理器
	// 职责：预热问卷缓存
	registry.Register(NewQuestionnairePublishedHandler(deps.Logger))

	// 6. 问卷下架事件处理器
	// 职责：清除问卷缓存
	registry.Register(NewQuestionnaireUnpublishedHandler(deps.Logger))

	// 7. 问卷归档事件处理器
	// 职责：清除所有版本缓存
	registry.Register(NewQuestionnaireArchivedHandler(deps.Logger))

	// ==================== 量表生命周期事件 ====================

	// 8. 量表发布事件处理器
	// 职责：预热缓存、预加载计算规则
	registry.Register(NewScalePublishedHandler(deps.Logger))

	// 9. 量表下架事件处理器
	// 职责：清除缓存和计算规则
	registry.Register(NewScaleUnpublishedHandler(deps.Logger))

	// 10. 量表归档事件处理器
	// 职责：清除所有版本缓存和规则
	registry.Register(NewScaleArchivedHandler(deps.Logger))

	// 11. 量表更新事件处理器
	// 职责：重新加载计算规则
	registry.Register(NewScaleUpdatedHandler(deps.Logger))
}
