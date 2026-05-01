package questionnaire

import (
	"context"
	stderrors "errors"
)

// ErrNotFound 表示问卷仓储未找到目标记录。
var ErrNotFound = stderrors.New("questionnaire not found")

// IsNotFound 判断错误是否为问卷仓储未找到。
func IsNotFound(err error) bool {
	return stderrors.Is(err, ErrNotFound)
}

// Repository 问卷存储库接口（出站端口）
// 定义了与存储相关的所有操作契约
type Repository interface {
	// 基础 CRUD 操作
	Create(ctx context.Context, qDomain *Questionnaire) error
	FindByCode(ctx context.Context, code string) (*Questionnaire, error)
	FindPublishedByCode(ctx context.Context, code string) (*Questionnaire, error)
	FindLatestPublishedByCode(ctx context.Context, code string) (*Questionnaire, error)
	FindByCodeVersion(ctx context.Context, code, version string) (*Questionnaire, error)
	FindBaseByCode(ctx context.Context, code string) (*Questionnaire, error)
	FindBasePublishedByCode(ctx context.Context, code string) (*Questionnaire, error)
	FindBaseByCodeVersion(ctx context.Context, code, version string) (*Questionnaire, error)
	LoadQuestions(ctx context.Context, qDomain *Questionnaire) error
	FindBaseList(ctx context.Context, page, pageSize int, conditions map[string]interface{}) ([]*Questionnaire, error)
	FindBasePublishedList(ctx context.Context, page, pageSize int, conditions map[string]interface{}) ([]*Questionnaire, error)
	CountWithConditions(ctx context.Context, conditions map[string]interface{}) (int64, error)
	CountPublishedWithConditions(ctx context.Context, conditions map[string]interface{}) (int64, error)
	Update(ctx context.Context, qDomain *Questionnaire) error
	CreatePublishedSnapshot(ctx context.Context, qDomain *Questionnaire, active bool) error
	SetActivePublishedVersion(ctx context.Context, code, version string) error
	ClearActivePublishedVersion(ctx context.Context, code string) error
	Remove(ctx context.Context, code string) error
	HardDelete(ctx context.Context, code string) error
	HardDeleteFamily(ctx context.Context, code string) error
	ExistsByCode(ctx context.Context, code string) (bool, error)
	HasPublishedSnapshots(ctx context.Context, code string) (bool, error)
}
