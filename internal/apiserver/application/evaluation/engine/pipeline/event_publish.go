package pipeline

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// EventPublishHandler 事件发布处理器
// 职责：发布 AssessmentInterpretedEvent 领域事件
// 位置：链尾，在所有处理器之后执行
// 输入：Context（包含完整的评估结果）
// 输出：发布事件到消息队列
//
// 事件消费方：
// - 通知服务：发送"报告已生成"通知给受试者
// - 预警服务：对高风险案例发送预警给相关人员
// - 统计服务：更新实时统计数据
type EventPublishHandler struct {
	*BaseHandler
	publisher event.EventPublisher
}

// EventPublishHandlerOption 事件发布处理器选项
type EventPublishHandlerOption func(*EventPublishHandler)

// NewEventPublishHandler 创建事件发布处理器
func NewEventPublishHandler(publisher event.EventPublisher, opts ...EventPublishHandlerOption) *EventPublishHandler {
	h := &EventPublishHandler{
		BaseHandler: NewBaseHandler("EventPublishHandler"),
		publisher:   publisher,
	}

	for _, opt := range opts {
		opt(h)
	}

	return h
}

// Handle 发布评估完成事件
func (h *EventPublishHandler) Handle(ctx context.Context, evalCtx *Context) error {
	// 检查前置条件
	if evalCtx.EvaluationResult == nil {
		evalCtx.SetError(ErrEvaluationResultRequired)
		return evalCtx.Error
	}

	// 构建领域事件
	domainEvent := h.buildEvent(evalCtx)

	// 发布事件
	if h.publisher != nil {
		if err := h.publisher.Publish(ctx, domainEvent); err != nil {
			// 事件发布失败不应该中断整个流程
			// 记录错误但继续执行（可以通过重试机制补偿）
			log.Warnf("failed to publish AssessmentInterpretedEvent for assessment %d: %v",
				evalCtx.Assessment.ID(), err)
		} else {
			log.Infof("AssessmentInterpretedEvent published for assessment %d, risk level: %s",
				evalCtx.Assessment.ID(), evalCtx.RiskLevel)
		}
	} else {
		log.Debugf("publisher is nil, skip event publishing for assessment %d", evalCtx.Assessment.ID())
	}

	// 同时将事件添加到 Assessment 的事件列表中（供仓储层使用）
	// 领域事件已在 Assessment.ApplyEvaluation 中添加

	// 继续下一个处理器
	return h.Next(ctx, evalCtx)
}

// buildEvent 构建评估完成事件
func (h *EventPublishHandler) buildEvent(evalCtx *Context) *assessment.AssessmentInterpretedEvent {
	a := evalCtx.Assessment
	result := evalCtx.EvaluationResult

	// 获取量表引用
	var scaleRef assessment.MedicalScaleRef
	if a.MedicalScaleRef() != nil {
		scaleRef = *a.MedicalScaleRef()
	}

	return assessment.NewAssessmentInterpretedEvent(
		a.ID(),
		a.TesteeID(),
		scaleRef,
		result.TotalScore,
		result.RiskLevel,
		time.Now(),
	)
}
