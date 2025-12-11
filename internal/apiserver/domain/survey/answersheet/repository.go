package answersheet

import (
	"context"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// AnswerSheetSummary 答卷摘要（用于列表展示，不包含答题详情）
type AnswerSheetSummary struct {
	ID                   meta.ID   // 答卷ID
	QuestionnaireCode    string    // 问卷编码
	QuestionnaireVersion string    // 问卷版本
	QuestionnaireTitle   string    // 问卷标题
	FillerID             uint64    // 填写者ID
	FillerType           string    // 填写者类型
	TotalScore           float64   // 总分
	AnswerCount          int       // 答案数量
	FilledAt             time.Time // 填写时间
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

	// FindSummaryListByFiller 查询填写者的答卷摘要列表（轻量级，不包含答题详情）
	FindSummaryListByFiller(ctx context.Context, fillerID uint64, page, pageSize int) ([]*AnswerSheetSummary, error)

	// FindSummaryListByQuestionnaire 查询问卷的答卷摘要列表（轻量级，不包含答题详情）
	FindSummaryListByQuestionnaire(ctx context.Context, questionnaireCode string, page, pageSize int) ([]*AnswerSheetSummary, error)

	// CountByFiller 统计填写者的答卷数量
	CountByFiller(ctx context.Context, fillerID uint64) (int64, error)

	// CountByQuestionnaire 统计问卷的答卷数量
	CountByQuestionnaire(ctx context.Context, questionnaireCode string) (int64, error)

	// CountWithConditions 根据条件统计数量
	CountWithConditions(ctx context.Context, conditions map[string]interface{}) (int64, error)

	// Delete 删除答卷
	Delete(ctx context.Context, id meta.ID) error
}
