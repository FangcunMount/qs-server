package composite

import (
	"context"
	"fmt"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/ports/storage"
)

// questionnaireCompositeRepository 组合问卷仓储
// 整合 MySQL（基础信息）和 MongoDB（文档结构）
type questionnaireCompositeRepository struct {
	mysqlRepo    storage.QuestionnaireRepository         // MySQL 仓储（基础信息）
	documentRepo storage.QuestionnaireDocumentRepository // MongoDB 仓储（文档结构）
}

// NewQuestionnaireCompositeRepository 创建组合问卷仓储
func NewQuestionnaireCompositeRepository(
	mysqlRepo storage.QuestionnaireRepository,
	documentRepo storage.QuestionnaireDocumentRepository,
) storage.QuestionnaireRepository {
	return &questionnaireCompositeRepository{
		mysqlRepo:    mysqlRepo,
		documentRepo: documentRepo,
	}
}

// Save 保存问卷（同时保存到 MySQL 和 MongoDB）
func (r *questionnaireCompositeRepository) Save(ctx context.Context, q *questionnaire.Questionnaire) error {
	// 1. 保存基础信息到 MySQL
	if err := r.mysqlRepo.Save(ctx, q); err != nil {
		return fmt.Errorf("failed to save questionnaire to MySQL: %w", err)
	}

	// 2. 保存文档结构到 MongoDB
	if err := r.documentRepo.SaveDocument(ctx, q); err != nil {
		// 如果 MongoDB 失败，尝试回滚 MySQL（简单实现）
		_ = r.mysqlRepo.Remove(ctx, q.ID())
		return fmt.Errorf("failed to save questionnaire document to MongoDB: %w", err)
	}

	return nil
}

// FindByID 根据ID查找问卷（从两个数据源合并数据）
func (r *questionnaireCompositeRepository) FindByID(ctx context.Context, id questionnaire.QuestionnaireID) (*questionnaire.Questionnaire, error) {
	// 1. 从 MySQL 获取基础信息
	baseQ, err := r.mysqlRepo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// 2. 从 MongoDB 获取文档结构
	_, err = r.documentRepo.GetDocument(ctx, id)
	if err != nil {
		// 如果文档不存在，返回基础问卷
		if err == questionnaire.ErrQuestionnaireNotFound {
			return baseQ, nil
		}
		return nil, fmt.Errorf("failed to get questionnaire document: %w", err)
	}

	// 3. 合并数据创建完整的问卷对象
	// TODO: 实现从存储数据重构领域对象的逻辑
	// 这里暂时返回基础问卷
	return baseQ, nil
}

// FindByCode 根据代码查找问卷
func (r *questionnaireCompositeRepository) FindByCode(ctx context.Context, code string) (*questionnaire.Questionnaire, error) {
	// 1. 从 MySQL 获取基础信息
	baseQ, err := r.mysqlRepo.FindByCode(ctx, code)
	if err != nil {
		return nil, err
	}

	// 2. 从 MongoDB 获取文档结构
	_, err = r.documentRepo.GetDocument(ctx, baseQ.ID())
	if err != nil {
		// 如果文档不存在，返回基础问卷
		if err == questionnaire.ErrQuestionnaireNotFound {
			return baseQ, nil
		}
		return nil, fmt.Errorf("failed to get questionnaire document: %w", err)
	}

	// 3. 合并数据
	// TODO: 实现数据合并逻辑
	return baseQ, nil
}

// Update 更新问卷（同时更新两个数据源）
func (r *questionnaireCompositeRepository) Update(ctx context.Context, q *questionnaire.Questionnaire) error {
	// 1. 更新 MySQL
	if err := r.mysqlRepo.Update(ctx, q); err != nil {
		return fmt.Errorf("failed to update questionnaire in MySQL: %w", err)
	}

	// 2. 更新 MongoDB
	if err := r.documentRepo.UpdateDocument(ctx, q); err != nil {
		return fmt.Errorf("failed to update questionnaire document in MongoDB: %w", err)
	}

	return nil
}

// Remove 删除问卷（从两个数据源删除）
func (r *questionnaireCompositeRepository) Remove(ctx context.Context, id questionnaire.QuestionnaireID) error {
	// 1. 删除 MongoDB 文档
	if err := r.documentRepo.RemoveDocument(ctx, id); err != nil {
		// 记录错误但继续删除 MySQL
		// log.Warnf("Failed to remove questionnaire document: %v", err)
	}

	// 2. 删除 MySQL 记录
	if err := r.mysqlRepo.Remove(ctx, id); err != nil {
		return fmt.Errorf("failed to remove questionnaire from MySQL: %w", err)
	}

	return nil
}

// FindPublishedQuestionnaires 查找已发布的问卷
func (r *questionnaireCompositeRepository) FindPublishedQuestionnaires(ctx context.Context) ([]*questionnaire.Questionnaire, error) {
	return r.mysqlRepo.FindPublishedQuestionnaires(ctx)
}

// FindQuestionnairesByCreator 根据创建者查找问卷
func (r *questionnaireCompositeRepository) FindQuestionnairesByCreator(ctx context.Context, creatorID string) ([]*questionnaire.Questionnaire, error) {
	return r.mysqlRepo.FindQuestionnairesByCreator(ctx, creatorID)
}

// FindQuestionnairesByStatus 根据状态查找问卷
func (r *questionnaireCompositeRepository) FindQuestionnairesByStatus(ctx context.Context, status questionnaire.Status) ([]*questionnaire.Questionnaire, error) {
	return r.mysqlRepo.FindQuestionnairesByStatus(ctx, status)
}

// FindQuestionnaires 分页查询问卷
func (r *questionnaireCompositeRepository) FindQuestionnaires(ctx context.Context, query storage.QueryOptions) (*storage.QuestionnaireQueryResult, error) {
	// 基础查询使用 MySQL
	result, err := r.mysqlRepo.FindQuestionnaires(ctx, query)
	if err != nil {
		return nil, err
	}

	// TODO: 可以在这里添加文档内容的搜索逻辑
	// 例如根据问题内容搜索等

	return result, nil
}

// ExistsByCode 检查代码是否存在
func (r *questionnaireCompositeRepository) ExistsByCode(ctx context.Context, code string) (bool, error) {
	return r.mysqlRepo.ExistsByCode(ctx, code)
}

// ExistsByID 检查ID是否存在
func (r *questionnaireCompositeRepository) ExistsByID(ctx context.Context, id questionnaire.QuestionnaireID) (bool, error) {
	return r.mysqlRepo.ExistsByID(ctx, id)
}
