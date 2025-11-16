package dto

import "github.com/FangcunMount/qs-server/internal/pkg/meta"

// AnswerSheetDTO 表示答卷数据传输对象
// 用于应用层和领域层之间的数据传输
type AnswerSheetDTO struct {
	ID                   meta.ID     // 答卷ID
	QuestionnaireCode    string      // 问卷代码
	QuestionnaireVersion string      // 问卷版本
	Title                string      // 答卷标题
	Score                float64     // 总分
	WriterID             uint64      // 填写人ID
	TesteeID             uint64      // 被测试者ID
	Answers              []AnswerDTO // 答案列表
}

// AnswerDTO 表示答案数据传输对象
// 用于在应用层和领域层之间传输单个答案数据
type AnswerDTO struct {
	QuestionCode string  // 问题编码
	QuestionType string  // 问题类型
	Score        float64 // 得分
	Value        any     // 答案值，可以是字符串、数字或选项数组等
}

// AnswerSheetDetailDTO 用于返回答卷详细信息的数据传输对象
type AnswerSheetDetailDTO struct {
	AnswerSheet   AnswerSheetDTO   // 答卷基本信息
	WriterName    string           // 填写人姓名
	TesteeName    string           // 被测试者姓名
	Questionnaire QuestionnaireDTO // 问卷信息
	CreatedAt     string           // 创建时间
	UpdatedAt     string           // 更新时间
}

// AnswerSheetStatisticsDTO 答卷统计数据传输对象
type AnswerSheetStatisticsDTO struct {
	QuestionnaireCode    string                  // 问卷代码
	QuestionnaireVersion string                  // 问卷版本
	TotalCount           int64                   // 答卷总数
	AverageScore         float64                 // 平均分
	QuestionStatistics   []QuestionStatisticsDTO // 题目统计
}

// QuestionStatisticsDTO 问题统计数据传输对象
type QuestionStatisticsDTO struct {
	QuestionCode       string           // 问题代码
	QuestionType       string           // 问题类型
	AverageScore       float64          // 平均分
	AnswerDistribution map[string]int64 // 答案分布（选项代码 -> 选择次数）
}
