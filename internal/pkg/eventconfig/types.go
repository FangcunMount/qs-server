package eventconfig

// ============================================================================
// 事件类型常量
// ============================================================================
// 这些常量与 configs/events.yaml 中的事件类型保持一致
// 使用常量而不是硬编码字符串，提供编译时检查和 IDE 自动补全
//
// 注意：添加新事件时，需要同时更新：
// 1. configs/events.yaml 中的事件配置
// 2. 本文件中的常量定义
// ============================================================================

// Survey 领域 - Questionnaire 聚合
const (
	// QuestionnairePublished 问卷已发布
	QuestionnairePublished = "questionnaire.published"
	// QuestionnaireUnpublished 问卷已下架
	QuestionnaireUnpublished = "questionnaire.unpublished"
	// QuestionnaireArchived 问卷已归档
	QuestionnaireArchived = "questionnaire.archived"
)

// Survey 领域 - AnswerSheet 聚合
const (
	// AnswerSheetSubmitted 答卷已提交
	AnswerSheetSubmitted = "answersheet.submitted"
)

// Evaluation 领域 - Assessment 聚合
const (
	// AssessmentSubmitted 测评已提交
	AssessmentSubmitted = "assessment.submitted"
	// AssessmentInterpreted 测评已解读
	AssessmentInterpreted = "assessment.interpreted"
	// AssessmentFailed 测评失败
	AssessmentFailed = "assessment.failed"
)

// Evaluation 领域 - Report 聚合
const (
	// ReportGenerated 报告已生成
	ReportGenerated = "report.generated"
	// ReportExported 报告已导出
	ReportExported = "report.exported"
)

// Scale 领域
const (
	// ScalePublished 量表已发布
	ScalePublished = "scale.published"
	// ScaleUnpublished 量表已下架
	ScaleUnpublished = "scale.unpublished"
	// ScaleUpdated 量表已更新
	ScaleUpdated = "scale.updated"
	// ScaleArchived 量表已归档
	ScaleArchived = "scale.archived"
)

// EventTypes 返回所有已注册的事件类型
// 用于验证配置文件中的事件类型是否完整
func EventTypes() []string {
	return []string{
		// Questionnaire
		QuestionnairePublished,
		QuestionnaireUnpublished,
		QuestionnaireArchived,
		// AnswerSheet
		AnswerSheetSubmitted,
		// Assessment
		AssessmentSubmitted,
		AssessmentInterpreted,
		AssessmentFailed,
		// Report
		ReportGenerated,
		ReportExported,
		// Scale
		ScalePublished,
		ScaleUnpublished,
		ScaleUpdated,
		ScaleArchived,
	}
}

// ValidateEventTypes 验证配置中是否包含所有已知事件类型
func ValidateEventTypes(cfg *Config) []string {
	var missing []string
	for _, et := range EventTypes() {
		if _, ok := cfg.Events[et]; !ok {
			missing = append(missing, et)
		}
	}
	return missing
}
