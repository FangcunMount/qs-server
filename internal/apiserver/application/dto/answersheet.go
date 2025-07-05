package dto

// AnswerSheetDTO 表示一个答卷结构（用于业务处理）
type AnswerSheetDTO struct {
	ID                   uint64 // 答卷ID（仅用于编辑）
	QuestionnaireCode    string
	QuestionnaireVersion string
	Title                string
	WriterID             uint64
	TesteeID             uint64
	Answers              []AnswerDTO
	Score                uint16
	CreatedAt            string
}

// OriginalAnswerSheetDTO 用于原始答卷组合
// 应该在 builder 中转为 interface 层 response
type OriginalAnswerSheetDTO struct {
	AnswerSheet   AnswerSheetDTO
	Questionnaire QuestionnaireDTO
	WriterName    string
	TesteeName    string
}

// AnswerDTO 表示一个题目的回答结构（用于 application 层）
type AnswerDTO struct {
	QuestionCode string // 问题编码
	QuestionType string // 问题类型（用于构造 AnswerValue）
	Answer       any    // 原始答案值（如 string/int/[]string）
	Score        uint16 // 单题得分（可选）
}
