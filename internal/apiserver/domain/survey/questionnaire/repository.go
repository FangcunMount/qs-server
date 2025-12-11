package questionnaire

import (
	"context"
)

// QuestionnaireSummary 问卷摘要（用于列表展示，不包含问题详情）
type QuestionnaireSummary struct {
	Code          string // 问卷编码
	Title         string // 问卷标题
	Description   string // 问卷描述
	ImgUrl        string // 封面图URL
	Version       string // 版本号
	Status        Status // 状态
	Type          QuestionnaireType // 问卷分类
	QuestionCount int    // 问题数量
}

// Repository 问卷存储库接口（出站端口）
// 定义了与存储相关的所有操作契约
type Repository interface {
	// 基础 CRUD 操作
	Create(ctx context.Context, qDomain *Questionnaire) error
	FindByCode(ctx context.Context, code string) (*Questionnaire, error)
	FindByCodeVersion(ctx context.Context, code, version string) (*Questionnaire, error)
	// FindSummaryList 查询问卷摘要列表（轻量级，不包含问题详情）
	// 注意：不再提供 FindList 返回完整问卷的方法，避免内存溢出
	FindSummaryList(ctx context.Context, page, pageSize int, conditions map[string]interface{}) ([]*QuestionnaireSummary, error)
	CountWithConditions(ctx context.Context, conditions map[string]interface{}) (int64, error)
	Update(ctx context.Context, qDomain *Questionnaire) error
	Remove(ctx context.Context, code string) error
	HardDelete(ctx context.Context, code string) error
	ExistsByCode(ctx context.Context, code string) (bool, error)
}
