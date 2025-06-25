package storage

import (
	"context"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire"
)

// QuestionnaireRepository 问卷仓储端口（业务契约）
// 定义了应用层对存储的需求，不涉及具体技术实现
type QuestionnaireRepository interface {
	// 基本 CRUD 操作
	Save(ctx context.Context, q *questionnaire.Questionnaire) error
	FindByID(ctx context.Context, id questionnaire.QuestionnaireID) (*questionnaire.Questionnaire, error)
	FindByCode(ctx context.Context, code string) (*questionnaire.Questionnaire, error)
	Update(ctx context.Context, q *questionnaire.Questionnaire) error
	Remove(ctx context.Context, id questionnaire.QuestionnaireID) error

	// 业务查询方法
	FindPublishedQuestionnaires(ctx context.Context) ([]*questionnaire.Questionnaire, error)
	FindQuestionnairesByCreator(ctx context.Context, creatorID string) ([]*questionnaire.Questionnaire, error)
	FindQuestionnairesByStatus(ctx context.Context, status questionnaire.Status) ([]*questionnaire.Questionnaire, error)

	// 分页查询
	FindQuestionnaires(ctx context.Context, query QueryOptions) (*QuestionnaireQueryResult, error)

	// 检查存在性
	ExistsByCode(ctx context.Context, code string) (bool, error)
	ExistsByID(ctx context.Context, id questionnaire.QuestionnaireID) (bool, error)
}

// QueryOptions 查询选项
type QueryOptions struct {
	Offset    int
	Limit     int
	CreatorID *string
	Status    *questionnaire.Status
	Keyword   *string
	SortBy    string
	SortOrder string
}

// QuestionnaireQueryResult 查询结果
type QuestionnaireQueryResult struct {
	Items      []*questionnaire.Questionnaire
	TotalCount int64
	HasMore    bool
}
