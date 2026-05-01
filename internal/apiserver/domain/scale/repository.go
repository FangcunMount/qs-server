package scale

import (
	"context"
	stderrors "errors"
)

// ErrNotFound 表示量表仓储未找到目标记录。
var ErrNotFound = stderrors.New("scale not found")

// IsNotFound 判断错误是否为量表仓储未找到。
func IsNotFound(err error) bool {
	return stderrors.Is(err, ErrNotFound)
}

// Repository 医学量表存储库接口（出站端口）
// 定义了与存储相关的所有操作契约
type Repository interface {
	// 基础 CRUD 操作
	Create(ctx context.Context, scale *MedicalScale) error
	FindByCode(ctx context.Context, code string) (*MedicalScale, error)
	FindByQuestionnaireCode(ctx context.Context, questionnaireCode string) (*MedicalScale, error)
	Update(ctx context.Context, scale *MedicalScale) error
	Remove(ctx context.Context, code string) error
	ExistsByCode(ctx context.Context, code string) (bool, error)
}

// HotScaleSummary 表示按填写热度聚合后的量表摘要。
type HotScaleSummary struct {
	Scale           *MedicalScale
	SubmissionCount int64
}
