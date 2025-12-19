package scale

import (
	"context"
)

// ScaleSummary 量表摘要（用于列表查询，不包含 factors 详情）
type ScaleSummary struct {
	Code              string
	Title             string
	Description       string
	Category          Category
	Stage             Stage
	ApplicableAge     ApplicableAge
	Reporters         []Reporter
	Tags              []Tag
	QuestionnaireCode string
	Status            Status
}

// Repository 医学量表存储库接口（出站端口）
// 定义了与存储相关的所有操作契约
type Repository interface {
	// 基础 CRUD 操作
	Create(ctx context.Context, scale *MedicalScale) error
	FindByCode(ctx context.Context, code string) (*MedicalScale, error)
	FindByQuestionnaireCode(ctx context.Context, questionnaireCode string) (*MedicalScale, error)
	// FindSummaryList 查询量表摘要列表（不包含 factors，用于列表展示）
	FindSummaryList(ctx context.Context, page, pageSize int, conditions map[string]string) ([]*ScaleSummary, error)
	CountWithConditions(ctx context.Context, conditions map[string]string) (int64, error)
	Update(ctx context.Context, scale *MedicalScale) error
	Remove(ctx context.Context, code string) error
	ExistsByCode(ctx context.Context, code string) (bool, error)
}
