package assessment

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// === Assessment 仓储 ===

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

// === AssessmentScore 仓储 ===

// ScoreRepository 测评得分仓储接口
type ScoreRepository interface {
	// === 批量保存 ===

	// SaveProjectionFromOutcome persists a query projection derived from one
	// immutable EvaluationOutcome. It is not an independent score fact.
	SaveProjectionFromOutcome(ctx context.Context, outcomeID meta.ID, assessmentDomain *Assessment, score *ScaleScoreProjection) error
}
