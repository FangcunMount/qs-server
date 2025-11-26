package questionnaire

import (
	"context"
)

// Repository 问卷存储库接口（出站端口）
// 定义了与存储相关的所有操作契约
type Repository interface {
	// 基础 CRUD 操作
	Create(ctx context.Context, qDomain *Questionnaire) error
	FindByCode(ctx context.Context, code string) (*Questionnaire, error)
	FindByCodeVersion(ctx context.Context, code, version string) (*Questionnaire, error)
	FindList(ctx context.Context, page, pageSize int, conditions map[string]string) ([]*Questionnaire, error)
	CountWithConditions(ctx context.Context, conditions map[string]string) (int64, error)
	Update(ctx context.Context, qDomain *Questionnaire) error
	Remove(ctx context.Context, code string) error
	HardDelete(ctx context.Context, code string) error
	ExistsByCode(ctx context.Context, code string) (bool, error)
	FindActiveQuestionnaires(ctx context.Context) ([]*Questionnaire, error)
}
