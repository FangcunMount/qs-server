package questionnaire

import (
	"time"

	"github.com/FangcunMount/qs-server/pkg/event"
)

// ==================== QuestionnairePublishedEvent ====================

// QuestionnairePublishedEvent 问卷已发布事件
// 用途：
// - collection-server 更新 Redis 缓存
// - 搜索服务更新索引
type QuestionnairePublishedEvent struct {
	event.BaseEvent

	questionnaireID uint64
	code            string
	version         string
	title           string
	publishedAt     time.Time
}

// NewQuestionnairePublishedEvent 创建问卷已发布事件
func NewQuestionnairePublishedEvent(
	questionnaireID uint64,
	code string,
	version string,
	title string,
	publishedAt time.Time,
) *QuestionnairePublishedEvent {
	return &QuestionnairePublishedEvent{
		BaseEvent:       event.NewBaseEvent("questionnaire.published", "Questionnaire", code),
		questionnaireID: questionnaireID,
		code:            code,
		version:         version,
		title:           title,
		publishedAt:     publishedAt,
	}
}

// QuestionnaireID 获取问卷ID
func (e *QuestionnairePublishedEvent) QuestionnaireID() uint64 {
	return e.questionnaireID
}

// Code 获取问卷编码
func (e *QuestionnairePublishedEvent) Code() string {
	return e.code
}

// Version 获取问卷版本
func (e *QuestionnairePublishedEvent) Version() string {
	return e.version
}

// Title 获取问卷标题
func (e *QuestionnairePublishedEvent) Title() string {
	return e.title
}

// PublishedAt 获取发布时间
func (e *QuestionnairePublishedEvent) PublishedAt() time.Time {
	return e.publishedAt
}

// ==================== QuestionnaireUnpublishedEvent ====================

// QuestionnaireUnpublishedEvent 问卷已下架事件
// 用途：
// - collection-server 清除 Redis 缓存
// - 搜索服务更新索引
type QuestionnaireUnpublishedEvent struct {
	event.BaseEvent

	questionnaireID uint64
	code            string
	version         string
	unpublishedAt   time.Time
}

// NewQuestionnaireUnpublishedEvent 创建问卷已下架事件
func NewQuestionnaireUnpublishedEvent(
	questionnaireID uint64,
	code string,
	version string,
	unpublishedAt time.Time,
) *QuestionnaireUnpublishedEvent {
	return &QuestionnaireUnpublishedEvent{
		BaseEvent:       event.NewBaseEvent("questionnaire.unpublished", "Questionnaire", code),
		questionnaireID: questionnaireID,
		code:            code,
		version:         version,
		unpublishedAt:   unpublishedAt,
	}
}

// QuestionnaireID 获取问卷ID
func (e *QuestionnaireUnpublishedEvent) QuestionnaireID() uint64 {
	return e.questionnaireID
}

// Code 获取问卷编码
func (e *QuestionnaireUnpublishedEvent) Code() string {
	return e.code
}

// Version 获取问卷版本
func (e *QuestionnaireUnpublishedEvent) Version() string {
	return e.version
}

// UnpublishedAt 获取下架时间
func (e *QuestionnaireUnpublishedEvent) UnpublishedAt() time.Time {
	return e.unpublishedAt
}

// ==================== QuestionnaireArchivedEvent ====================

// QuestionnaireArchivedEvent 问卷已归档事件
// 用途：
// - collection-server 清除 Redis 缓存
// - 归档问卷不再对外提供服务
type QuestionnaireArchivedEvent struct {
	event.BaseEvent

	questionnaireID uint64
	code            string
	version         string
	archivedAt      time.Time
}

// NewQuestionnaireArchivedEvent 创建问卷已归档事件
func NewQuestionnaireArchivedEvent(
	questionnaireID uint64,
	code string,
	version string,
	archivedAt time.Time,
) *QuestionnaireArchivedEvent {
	return &QuestionnaireArchivedEvent{
		BaseEvent:       event.NewBaseEvent("questionnaire.archived", "Questionnaire", code),
		questionnaireID: questionnaireID,
		code:            code,
		version:         version,
		archivedAt:      archivedAt,
	}
}

// QuestionnaireID 获取问卷ID
func (e *QuestionnaireArchivedEvent) QuestionnaireID() uint64 {
	return e.questionnaireID
}

// Code 获取问卷编码
func (e *QuestionnaireArchivedEvent) Code() string {
	return e.code
}

// Version 获取问卷版本
func (e *QuestionnaireArchivedEvent) Version() string {
	return e.version
}

// ArchivedAt 获取归档时间
func (e *QuestionnaireArchivedEvent) ArchivedAt() time.Time {
	return e.archivedAt
}
