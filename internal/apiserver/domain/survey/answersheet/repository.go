package answersheet

import (
	"context"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// AnswerSheetSummary 是旧 Mongo 读模型的过渡行结构。
// Deprecated: read-side callers should use port/surveyreadmodel.AnswerSheetSummaryRow.
type AnswerSheetSummary struct {
	ID                   meta.ID
	QuestionnaireCode    string
	QuestionnaireVersion string
	QuestionnaireTitle   string
	FillerID             uint64
	FillerType           string
	TotalScore           float64
	AnswerCount          int
	FilledAt             time.Time
}

// Repository 答卷仓储接口（出站端口）
// 定义了与存储相关的所有操作契约
type Repository interface {
	// Create 创建答卷
	Create(ctx context.Context, sheet *AnswerSheet) error

	// Update 更新答卷
	Update(ctx context.Context, sheet *AnswerSheet) error

	// FindByID 根据 ID 查询答卷（返回完整答卷，包含所有答题）
	FindByID(ctx context.Context, id meta.ID) (*AnswerSheet, error)

	// Delete 删除答卷
	Delete(ctx context.Context, id meta.ID) error
}
