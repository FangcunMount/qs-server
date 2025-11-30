package core

import (
	"context"
)

// ==================== Topic 定义 ====================
// 集中管理所有 NSQ Topic 的定义，便于维护和查找

const (
	// TopicQuestionnaireLifecycle 问卷生命周期 Topic
	// 包含事件：问卷发布、下架、归档；量表发布、下架、归档、更新
	TopicQuestionnaireLifecycle = "questionnaire.lifecycle"

	// TopicAssessmentLifecycle 测评生命周期 Topic
	// 包含事件：答卷保存、测评提交、测评解读完成、测评失败
	TopicAssessmentLifecycle = "assessment.lifecycle"
)

// ==================== 事件类型定义 ====================
// 定义各 Topic 下的事件类型

// 问卷生命周期事件
const (
	EventQuestionnairePublished   = "questionnaire.published"
	EventQuestionnaireUnpublished = "questionnaire.unpublished"
	EventQuestionnaireArchived    = "questionnaire.archived"
)

// 量表生命周期事件
const (
	EventScalePublished   = "scale.published"
	EventScaleUnpublished = "scale.unpublished"
	EventScaleArchived    = "scale.archived"
	EventScaleUpdated     = "scale.updated"
)

// 测评生命周期事件
const (
	EventAnswerSheetSaved      = "answersheet.saved"
	EventAssessmentSubmitted   = "assessment.submitted"
	EventAssessmentInterpreted = "assessment.interpreted"
	EventAssessmentFailed      = "assessment.failed"
)

// ==================== 核心接口定义 ====================

// MessageHandler 消息处理器接口
// 每个具体事件类型实现这个接口，处理该类型的消息
type MessageHandler interface {
	// EventType 返回事件类型（如 "questionnaire.published"）
	EventType() string
	// Handle 处理消息
	Handle(ctx context.Context, payload []byte) error
}

// TopicHandler Topic 级别事件处理器接口
// 每个 TopicHandler 订阅一个 NSQ Topic，处理该 Topic 下的所有消息
type TopicHandler interface {
	// Handle 处理消息
	Handle(ctx context.Context, payload []byte) error
	// Topic 返回处理器订阅的主题
	Topic() string
	// Name 返回处理器名称
	Name() string
}

// ==================== Topic 配置 ====================

// TopicConfig Topic 配置
type TopicConfig struct {
	Topic       string // Topic 名称
	HandlerName string // Handler 名称
	Description string // 描述
}

// AllTopicConfigs 所有 Topic 配置
var AllTopicConfigs = []TopicConfig{
	{
		Topic:       TopicQuestionnaireLifecycle,
		HandlerName: "questionnaire_lifecycle_handler",
		Description: "问卷和量表生命周期事件",
	},
	{
		Topic:       TopicAssessmentLifecycle,
		HandlerName: "assessment_lifecycle_handler",
		Description: "测评生命周期事件",
	},
}
