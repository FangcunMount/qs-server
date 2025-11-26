package answersheet

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// Repository 答卷仓储接口（出站端口）
// 定义了与存储相关的所有操作契约
type Repository interface {
	// Create 创建答卷
	Create(ctx context.Context, sheet *AnswerSheet) error

	// Update 更新答卷
	Update(ctx context.Context, sheet *AnswerSheet) error

	// FindByID 根据 ID 查询答卷
	FindByID(ctx context.Context, id meta.ID) (*AnswerSheet, error)

	// FindListByFiller 查询填写者的答卷列表
	FindListByFiller(ctx context.Context, fillerID uint64, page, pageSize int) ([]*AnswerSheet, error)

	// FindListByQuestionnaire 查询问卷的答卷列表
	FindListByQuestionnaire(ctx context.Context, questionnaireCode string, page, pageSize int) ([]*AnswerSheet, error)

	// CountWithConditions 根据条件统计数量
	CountWithConditions(ctx context.Context, conditions map[string]interface{}) (int64, error)

	// Delete 删除答卷
	Delete(ctx context.Context, id meta.ID) error
}
