package answersheet

import (
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
)

// ============= Result 定义 =============
// Results 用于应用服务层的输出结果

// AnswerSheetResult 答卷结果
type AnswerSheetResult struct {
	ID                 uint64         // 答卷ID
	QuestionnaireCode  string         // 问卷编码
	QuestionnaireVer   string         // 问卷版本
	QuestionnaireTitle string         // 问卷标题
	FillerID           uint64         // 填写人ID
	FillerName         string         // 填写人姓名
	FilledAt           time.Time      // 填写时间
	Score              float64        // 总分
	Answers            []AnswerResult // 答案列表
}

// AnswerResult 答案结果
type AnswerResult struct {
	QuestionCode string      // 问题编码
	QuestionType string      // 问题类型
	Value        interface{} // 答案值
	Score        float64     // 得分
}

// AnswerSheetListResult 答卷列表结果
type AnswerSheetListResult struct {
	Items []*AnswerSheetResult // 答卷列表
	Total int64                // 总数
}

// AnswerSheetStatistics 答卷统计结果
type AnswerSheetStatistics struct {
	QuestionnaireCode string  // 问卷编码
	TotalCount        int64   // 答卷总数
	AverageScore      float64 // 平均分
	MaxScore          float64 // 最高分
	MinScore          float64 // 最低分
}

// ============= Converter 转换器 =============

// toAnswerSheetResult 将领域模型转换为结果对象
func toAnswerSheetResult(as *answersheet.AnswerSheet) *AnswerSheetResult {
	if as == nil {
		return nil
	}

	// 获取问卷信息（返回三个值：code, version, title）
	qCode, qVersion, qTitle := as.QuestionnaireInfo()

	result := &AnswerSheetResult{
		ID:                 uint64(as.ID()),
		QuestionnaireCode:  qCode,
		QuestionnaireVer:   qVersion,
		QuestionnaireTitle: qTitle,
		FilledAt:           as.FilledAt(),
		Score:              as.Score(),
		Answers:            make([]AnswerResult, 0),
	}

	// 填写人信息
	if filler := as.Filler(); filler != nil {
		result.FillerID = uint64(filler.UserID())
		// FillerRef 没有 Name 方法，需要从其他地方获取或省略
		// TODO: 如果需要显示姓名，需要根据 UserID 查询
		result.FillerName = "" // 暂时留空
	}

	// 转换答案列表
	for _, answer := range as.Answers() {
		result.Answers = append(result.Answers, AnswerResult{
			QuestionCode: answer.QuestionCode(),
			QuestionType: answer.QuestionType(),
			Value:        answer.Value().Raw(),
			Score:        answer.Score(),
		})
	}

	return result
}

// toAnswerSheetListResult 将答卷列表转换为结果对象
func toAnswerSheetListResult(items []*answersheet.AnswerSheet, total int64) *AnswerSheetListResult {
	result := &AnswerSheetListResult{
		Items: make([]*AnswerSheetResult, 0, len(items)),
		Total: total,
	}

	for _, item := range items {
		result.Items = append(result.Items, toAnswerSheetResult(item))
	}

	return result
}
