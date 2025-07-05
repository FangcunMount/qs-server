package port

import (
	"context"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/answersheet"
)

// AnswerSheetRepositoryMongo 答卷存储库接口（出站端口）
// 定义了与存储相关的所有操作契约
type AnswerSheetRepositoryMongo interface {
	Create(ctx context.Context, aDomain *answersheet.AnswerSheet) error
	FindByID(ctx context.Context, id uint64) (*answersheet.AnswerSheet, error)
	FindListByWriter(ctx context.Context, writerID uint64, page, pageSize int) ([]*answersheet.AnswerSheet, error)
	FindListByTestee(ctx context.Context, testeeID uint64, page, pageSize int) ([]*answersheet.AnswerSheet, error)
	CountWithConditions(ctx context.Context, conditions map[string]interface{}) (int64, error)
}
