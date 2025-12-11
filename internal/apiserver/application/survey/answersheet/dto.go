package answersheet

import "time"

// ============= DTO 定义 =============
// DTOs 用于应用服务层的输入参数

// SubmitAnswerSheetDTO 提交答卷 DTO
type SubmitAnswerSheetDTO struct {
	QuestionnaireCode string      // 问卷编码
	QuestionnaireVer  string      // 问卷版本（如 "1.0.1"，空字符串表示使用最新版本）
	FillerID          uint64      // 填写人ID
	Answers           []AnswerDTO // 答案列表
}

// AnswerDTO 答案 DTO
type AnswerDTO struct {
	QuestionCode string      // 问题编码
	QuestionType string      // 问题类型
	Value        interface{} // 答案值（根据问题类型可能是string、number、[]string等）
}

// ListMyAnswerSheetsDTO 查询我的答卷列表 DTO
type ListMyAnswerSheetsDTO struct {
	FillerID          uint64            // 填写人ID（必填）
	QuestionnaireCode string            // 问卷编码（可选，筛选特定问卷的答卷）
	Page              int               // 页码
	PageSize          int               // 每页数量
	Conditions        map[string]string // 其他查询条件
}

// ListAnswerSheetsDTO 查询答卷列表 DTO（管理员用）
type ListAnswerSheetsDTO struct {
	QuestionnaireCode string            // 问卷编码（可选）
	FillerID          *uint64           // 填写人ID（可选）
	StartTime         *time.Time        // 开始时间（可选）
	EndTime           *time.Time        // 结束时间（可选）
	Page              int               // 页码
	PageSize          int               // 每页数量
	Conditions        map[string]string // 其他查询条件
}
