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
	// QuestionnaireChanged 问卷生命周期变化
	QuestionnaireChanged = "questionnaire.changed"
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
)

// Analytics / Behavior 领域
const (
	// FootprintEntryOpened 用户成功打开入口
	FootprintEntryOpened = "footprint.entry_opened"
	// FootprintIntakeConfirmed 完成一次接入
	FootprintIntakeConfirmed = "footprint.intake_confirmed"
	// FootprintTesteeProfileCreated 新建受试者档案
	FootprintTesteeProfileCreated = "footprint.testee_profile_created"
	// FootprintCareRelationshipEstablished 建立看护/服务关系
	FootprintCareRelationshipEstablished = "footprint.care_relationship_established"
	// FootprintCareRelationshipTransferred 转移看护关系
	FootprintCareRelationshipTransferred = "footprint.care_relationship_transferred"
	// FootprintAnswerSheetSubmitted 提交答卷
	FootprintAnswerSheetSubmitted = "footprint.answersheet_submitted"
	// FootprintAssessmentCreated 形成测评
	FootprintAssessmentCreated = "footprint.assessment_created"
	// FootprintReportGenerated 产出报告
	FootprintReportGenerated = "footprint.report_generated"
)

// Scale 领域
const (
	// ScaleChanged 量表生命周期变化
	ScaleChanged = "scale.changed"
)

// Plan 领域 - AssessmentTask 聚合
const (
	// TaskOpened 任务已开放
	TaskOpened = "task.opened"
	// TaskCompleted 任务已完成
	TaskCompleted = "task.completed"
	// TaskExpired 任务已过期
	TaskExpired = "task.expired"
	// TaskCanceled 任务已取消
	TaskCanceled = "task.canceled"
)

// EventTypes 返回所有已注册的事件类型
// 用于验证配置文件中的事件类型是否完整
func EventTypes() []string {
	return []string{
		// Questionnaire
		QuestionnaireChanged,
		// AnswerSheet
		AnswerSheetSubmitted,
		// Assessment
		AssessmentSubmitted,
		AssessmentInterpreted,
		AssessmentFailed,
		// Report
		ReportGenerated,
		// Behavior footprint
		FootprintEntryOpened,
		FootprintIntakeConfirmed,
		FootprintTesteeProfileCreated,
		FootprintCareRelationshipEstablished,
		FootprintCareRelationshipTransferred,
		FootprintAnswerSheetSubmitted,
		FootprintAssessmentCreated,
		FootprintReportGenerated,
		// Scale
		ScaleChanged,
		// Task
		TaskOpened,
		TaskCompleted,
		TaskExpired,
		TaskCanceled,
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
