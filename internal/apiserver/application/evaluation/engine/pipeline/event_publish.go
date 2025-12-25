package pipeline

import (
	"context"
	"strconv"
	"time"

	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/report"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/waiter"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// EventPublishHandler 事件发布处理器
// 职责：发布评估相关的领域事件
// 位置：链尾，在所有处理器之后执行
// 输入：Context（包含完整的评估结果和报告）
// 输出：发布事件到消息队列
//
// 发布的事件：
// - AssessmentInterpretedEvent：测评已解读事件
// - ReportGeneratedEvent：报告已生成事件（如果报告已生成）
//
// 事件消费方：
// - 通知服务：发送"报告已生成"通知给受试者
// - 预警服务：对高风险案例发送预警给相关人员
// - 统计服务：更新实时统计数据
type EventPublishHandler struct {
	*BaseHandler
	publisher     event.EventPublisher
	waiterRegistry *waiter.WaiterRegistry
}

// EventPublishHandlerOption 事件发布处理器选项
type EventPublishHandlerOption func(*EventPublishHandler)

// WithWaiterRegistry 设置等待队列注册表
func WithWaiterRegistry(waiterRegistry *waiter.WaiterRegistry) EventPublishHandlerOption {
	return func(h *EventPublishHandler) {
		h.waiterRegistry = waiterRegistry
	}
}

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

	// 发布 assessment.interpreted 事件
	h.publishAssessmentInterpretedEvent(ctx, evalCtx)

	// 发布 report.generated 事件（如果报告已生成）
	if evalCtx.Report != nil {
		h.publishReportGeneratedEvent(ctx, evalCtx)
	}

	// 继续下一个处理器
	return h.Next(ctx, evalCtx)
}

// publishAssessmentInterpretedEvent 发布测评已解读事件
func (h *EventPublishHandler) publishAssessmentInterpretedEvent(ctx context.Context, evalCtx *Context) {
	if h.publisher == nil {
		return
	}

	l := logger.L(ctx)
	a := evalCtx.Assessment
	result := evalCtx.EvaluationResult

	// 获取量表引用
	var scaleRef assessment.MedicalScaleRef
	if a.MedicalScaleRef() != nil {
		scaleRef = *a.MedicalScaleRef()
	}

	// 构建事件
	domainEvent := assessment.NewAssessmentInterpretedEvent(
		a.ID(),
		a.TesteeID(),
		scaleRef,
		result.TotalScore,
		result.RiskLevel,
		time.Now(),
	)

	// 发布事件
	if err := h.publisher.Publish(ctx, domainEvent); err != nil {
		l.Warnw("failed to publish AssessmentInterpretedEvent",
			"action", "publish_event",
			"assessment_id", evalCtx.Assessment.ID(),
			"result", "failed",
			"error", err.Error(),
		)
	} else {
		l.Infow("AssessmentInterpretedEvent published",
			"action", "publish_event",
			"assessment_id", evalCtx.Assessment.ID(),
			"risk_level", evalCtx.RiskLevel,
			"result", "success",
		)
	}

	// 通知等待队列（长轮询机制）
	if h.waiterRegistry != nil {
		assessmentID := a.ID().Uint64()
		riskLevelStr := string(result.RiskLevel)
		summary := waiter.StatusSummary{
			Status:     "interpreted",
			TotalScore: &result.TotalScore,
			RiskLevel:  &riskLevelStr,
			UpdatedAt:  time.Now().Unix(),
		}
		h.waiterRegistry.Notify(ctx, assessmentID, summary)
		l.Debugw("notified waiters for assessment",
			"assessment_id", assessmentID,
			"waiter_count", h.waiterRegistry.GetWaiterCount(assessmentID),
		)
	}
}

// publishReportGeneratedEvent 发布报告生成事件
func (h *EventPublishHandler) publishReportGeneratedEvent(ctx context.Context, evalCtx *Context) {
	if h.publisher == nil {
		return
	}

	l := logger.L(ctx)
	rpt := evalCtx.Report
	reportID := rpt.ID().Uint64()
	assessmentID := evalCtx.Assessment.ID().Uint64()
	testeeID := uint64(evalCtx.Assessment.TesteeID())

	// 获取量表信息
	var scaleCode, scaleVersion string
	if evalCtx.MedicalScale != nil {
		scaleCode = evalCtx.MedicalScale.GetCode().String()
		scaleVersion = evalCtx.MedicalScale.GetQuestionnaireVersion()
	} else if evalCtx.Assessment.MedicalScaleRef() != nil {
		scaleCode = evalCtx.Assessment.MedicalScaleRef().Code().String()
		// MedicalScaleRef 没有版本信息，使用问卷版本
		questionnaireRef := evalCtx.Assessment.QuestionnaireRef()
		if !questionnaireRef.IsEmpty() {
			scaleVersion = questionnaireRef.Version()
		}
	}

	// 构建事件
	domainEvent := domainReport.NewReportGeneratedEvent(
		strconv.FormatUint(reportID, 10),
		strconv.FormatUint(assessmentID, 10),
		testeeID,
		scaleCode,
		scaleVersion,
		evalCtx.TotalScore,
		string(evalCtx.RiskLevel),
		time.Now(),
	)

	// 发布事件
	if err := h.publisher.Publish(ctx, domainEvent); err != nil {
		l.Warnw("failed to publish ReportGeneratedEvent",
			"action", "publish_event",
			"report_id", reportID,
			"assessment_id", assessmentID,
			"result", "failed",
			"error", err.Error(),
		)
	} else {
		l.Infow("ReportGeneratedEvent published",
			"action", "publish_event",
			"report_id", reportID,
			"assessment_id", assessmentID,
			"testee_id", testeeID,
			"risk_level", evalCtx.RiskLevel,
			"result", "success",
		)
	}
}
