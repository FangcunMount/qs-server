package event

// ==================== 领域事件类型索引 ====================
//
// DEPRECATED: 此文件仅作为文档索引使用。
// 各领域事件类型常量应定义在各自的领域包中，遵循"领域事件由所属限界上下文自举"原则。
//
// 事件类型定义位置:
//   - questionnaire.EventType*  → internal/apiserver/domain/survey/questionnaire/events.go
//   - answersheet.EventType*    → internal/apiserver/domain/survey/answersheet/events.go
//   - scale.EventType*          → internal/apiserver/domain/scale/events.go
//   - assessment.EventType*     → internal/apiserver/domain/evaluation/assessment/events.go
//   - report.EventType*         → internal/apiserver/domain/evaluation/report/events.go
//
// 事件格式规范:
//   - 事件类型: {aggregate}.{action}，如 "questionnaire.published"
//   - 聚合类型: 聚合根名称，如 "Questionnaire"
//   - 聚合ID:   实体的数据库主键 ID（字符串形式）
//
// 事件载荷规范:
//   - 所有字段必须导出（首字母大写）
//   - 所有字段必须添加 JSON tag
//   - 避免嵌套领域对象，使用扁平化的可序列化字段
//
// 示例:
//   type QuestionnairePublishedEvent struct {
//       event.BaseEvent
//       QuestionnaireID int64  `json:"questionnaire_id"`
//       Code            string `json:"code"`
//       Version         int    `json:"version"`
//   }

// ==================== 事件来源常量 ====================
// 标识事件产生的服务，供基础设施层使用

const (
	SourceAPIServer        = "api-server"
	SourceCollectionServer = "collection-server"
	SourceWorker           = "qs-worker"
)
