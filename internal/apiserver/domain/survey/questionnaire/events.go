package questionnaire

import (
	"strconv"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/eventconfig"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// ==================== 事件类型常量 ====================
// 从 eventconfig 包导入，保持事件类型的单一来源

const (
	// EventTypePublished 问卷已发布
	EventTypePublished = eventconfig.QuestionnairePublished
	// EventTypeUnpublished 问卷已下架
	EventTypeUnpublished = eventconfig.QuestionnaireUnpublished
	// EventTypeArchived 问卷已归档
	EventTypeArchived = eventconfig.QuestionnaireArchived
)

// AggregateType 聚合根类型
const AggregateType = "Questionnaire"

// ==================== 事件 Payload 定义 ====================

// QuestionnairePublishedData 问卷已发布事件数据
type QuestionnairePublishedData struct {
	QuestionnaireID uint64    `json:"questionnaire_id"`
	Code            string    `json:"code"`
	Version         string    `json:"version"`
	Title           string    `json:"title"`
	PublishedAt     time.Time `json:"published_at"`
}

// QuestionnaireUnpublishedData 问卷已下架事件数据
type QuestionnaireUnpublishedData struct {
	QuestionnaireID uint64    `json:"questionnaire_id"`
	Code            string    `json:"code"`
	Version         string    `json:"version"`
	UnpublishedAt   time.Time `json:"unpublished_at"`
}

// QuestionnaireArchivedData 问卷已归档事件数据
type QuestionnaireArchivedData struct {
	QuestionnaireID uint64    `json:"questionnaire_id"`
	Code            string    `json:"code"`
	Version         string    `json:"version"`
	ArchivedAt      time.Time `json:"archived_at"`
}

// ==================== 事件类型别名 ====================

// QuestionnairePublishedEvent 问卷已发布事件
type QuestionnairePublishedEvent = event.Event[QuestionnairePublishedData]

// QuestionnaireUnpublishedEvent 问卷已下架事件
type QuestionnaireUnpublishedEvent = event.Event[QuestionnaireUnpublishedData]

// QuestionnaireArchivedEvent 问卷已归档事件
type QuestionnaireArchivedEvent = event.Event[QuestionnaireArchivedData]

// ==================== 事件构造函数 ====================

// NewQuestionnairePublishedEvent 创建问卷已发布事件
func NewQuestionnairePublishedEvent(
	questionnaireID uint64,
	code string,
	version string,
	title string,
	publishedAt time.Time,
) QuestionnairePublishedEvent {
	return event.New(EventTypePublished, AggregateType, strconv.FormatUint(questionnaireID, 10),
		QuestionnairePublishedData{
			QuestionnaireID: questionnaireID,
			Code:            code,
			Version:         version,
			Title:           title,
			PublishedAt:     publishedAt,
		},
	)
}

// NewQuestionnaireUnpublishedEvent 创建问卷已下架事件
func NewQuestionnaireUnpublishedEvent(
	questionnaireID uint64,
	code string,
	version string,
	unpublishedAt time.Time,
) QuestionnaireUnpublishedEvent {
	return event.New(EventTypeUnpublished, AggregateType, strconv.FormatUint(questionnaireID, 10),
		QuestionnaireUnpublishedData{
			QuestionnaireID: questionnaireID,
			Code:            code,
			Version:         version,
			UnpublishedAt:   unpublishedAt,
		},
	)
}

// NewQuestionnaireArchivedEvent 创建问卷已归档事件
func NewQuestionnaireArchivedEvent(
	questionnaireID uint64,
	code string,
	version string,
	archivedAt time.Time,
) QuestionnaireArchivedEvent {
	return event.New(EventTypeArchived, AggregateType, strconv.FormatUint(questionnaireID, 10),
		QuestionnaireArchivedData{
			QuestionnaireID: questionnaireID,
			Code:            code,
			Version:         version,
			ArchivedAt:      archivedAt,
		},
	)
}
