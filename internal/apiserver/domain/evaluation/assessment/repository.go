package assessment

import "context"

// ==================== Assessment Repository ====================

// Repository 测评仓储接口（出站端口）
type Repository interface {
	// === 基础 CRUD ===

	// Save 保存测评（新增或更新）
	Save(ctx context.Context, assessment *Assessment) error

	// FindByID 根据ID查找
	FindByID(ctx context.Context, id ID) (*Assessment, error)

	// Delete 删除测评
	Delete(ctx context.Context, id ID) error

	// === 按关联查询 ===

	// FindByAnswerSheetID 根据答卷ID查找
	FindByAnswerSheetID(ctx context.Context, answerSheetID AnswerSheetRef) (*Assessment, error)
}

// ==================== AssessmentScore Repository ====================

// ScoreRepository 测评得分仓储接口
type ScoreRepository interface {
	// === 批量保存 ===

	// SaveScoresWithContext 带上下文保存得分（包含受试者和量表信息）
	// 需要传入 Assessment 对象来获取必要的辅助信息（testeeID, scaleID 等）
	SaveScoresWithContext(ctx context.Context, assessmentDomain *Assessment, score *AssessmentScore) error

	// === 基础查询 ===

	// === 删除 ===

	// DeleteByAssessmentID 删除测评的所有得分
	DeleteByAssessmentID(ctx context.Context, assessmentID ID) error
}
