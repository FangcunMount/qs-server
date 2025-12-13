// Package pipeline 评估管道
// 使用职责链模式实现评估流程，让一个评估消息被多个处理器（模块）依次消费。
//
// 设计说明：
// 每个处理器负责一个独立的职责，遵循单一职责原则。
//
// 处理器链路：
//  1. ValidationHandler - 前置校验（校验输入数据完整性）
//  2. FactorScoreHandler - 因子分数计算（从答卷读取预计算分数，按因子聚合，计算总分）
//  3. RiskLevelHandler - 风险等级计算（计算因子/整体风险等级，保存得分）
//  4. InterpretationHandler - 测评分析解读、保存（生成结论建议，保存报告）
//  5. EventPublishHandler - 事件发布（发布 AssessmentInterpretedEvent）
//
// 扩展性：
// - 新增处理器只需实现 Handler 接口
// - 通过 Chain.AddHandler 添加到链路中
// - 各处理器可独立测试和复用
package pipeline

import "context"

// Handler 评估处理器接口
// 职责链模式的核心接口，每个处理器负责评估流程中的一个环节
type Handler interface {
	// Handle 处理评估请求
	// ctx: Go context，用于超时和取消控制
	// evalCtx: 评估上下文，携带评估数据和中间结果
	// 返回 error 表示处理失败，会中断整个链路
	Handle(ctx context.Context, evalCtx *Context) error

	// SetNext 设置下一个处理器
	SetNext(handler Handler) Handler

	// Name 处理器名称（用于日志和调试）
	Name() string
}

// BaseHandler 基础处理器
// 提供职责链的默认实现，具体处理器可以嵌入此结构体
type BaseHandler struct {
	next Handler
	name string
}

// NewBaseHandler 创建基础处理器
func NewBaseHandler(name string) *BaseHandler {
	return &BaseHandler{name: name}
}

// SetNext 设置下一个处理器
func (h *BaseHandler) SetNext(handler Handler) Handler {
	h.next = handler
	return handler
}

// Next 调用下一个处理器
func (h *BaseHandler) Next(ctx context.Context, evalCtx *Context) error {
	if h.next != nil {
		return h.next.Handle(ctx, evalCtx)
	}
	return nil
}

// Name 获取处理器名称
func (h *BaseHandler) Name() string {
	return h.name
}
