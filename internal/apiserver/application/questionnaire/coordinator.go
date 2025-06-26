package questionnaire

import (
	"context"
	"fmt"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/ports/storage"
)

// DataCoordinator 问卷数据协调器
// 负责协调 MySQL（基础信息）和 MongoDB（文档结构）的数据操作
// 这是应用服务层的组件，处理跨数据源的业务逻辑
type DataCoordinator struct {
	mysqlRepo    storage.QuestionnaireRepository         // MySQL 仓储（基础信息）
	documentRepo storage.QuestionnaireDocumentRepository // MongoDB 仓储（文档结构）
}

// NewDataCoordinator 创建数据协调器
func NewDataCoordinator(
	mysqlRepo storage.QuestionnaireRepository,
	documentRepo storage.QuestionnaireDocumentRepository,
) *DataCoordinator {
	return &DataCoordinator{
		mysqlRepo:    mysqlRepo,
		documentRepo: documentRepo,
	}
}

// SaveQuestionnaire 保存问卷（协调多个数据源）
func (c *DataCoordinator) SaveQuestionnaire(ctx context.Context, q *questionnaire.Questionnaire) error {
	// 业务规则：先保存基础信息，再保存文档结构

	// 1. 保存基础信息到 MySQL
	if err := c.mysqlRepo.Save(ctx, q); err != nil {
		return fmt.Errorf("failed to save questionnaire basic info: %w", err)
	}

	// 2. 保存文档结构到 MongoDB
	if err := c.documentRepo.SaveDocument(ctx, q); err != nil {
		// 应用层处理事务一致性：回滚MySQL操作
		if rollbackErr := c.mysqlRepo.Remove(ctx, q.ID()); rollbackErr != nil {
			// 记录回滚失败，这是严重的数据一致性问题
			return fmt.Errorf("failed to save document and rollback failed: original=%w, rollback=%w", err, rollbackErr)
		}
		return fmt.Errorf("failed to save questionnaire document: %w", err)
	}

	return nil
}

// GetCompleteQuestionnaire 获取完整的问卷（合并多个数据源）
func (c *DataCoordinator) GetCompleteQuestionnaire(ctx context.Context, id questionnaire.QuestionnaireID) (*questionnaire.Questionnaire, error) {
	// 1. 从 MySQL 获取基础信息
	baseQ, err := c.mysqlRepo.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get questionnaire basic info: %w", err)
	}

	// 2. 从 MongoDB 获取文档结构
	document, err := c.documentRepo.GetDocument(ctx, id)
	if err != nil {
		// 业务规则：如果文档不存在，返回基础问卷（向后兼容）
		if err == questionnaire.ErrQuestionnaireNotFound {
			return baseQ, nil
		}
		return nil, fmt.Errorf("failed to get questionnaire document: %w", err)
	}

	// 3. 应用层负责数据合并逻辑
	completeQ, err := c.mergeQuestionnaireData(baseQ, document)
	if err != nil {
		return nil, fmt.Errorf("failed to merge questionnaire data: %w", err)
	}

	return completeQ, nil
}

// UpdateQuestionnaire 更新问卷（协调多个数据源）
func (c *DataCoordinator) UpdateQuestionnaire(ctx context.Context, q *questionnaire.Questionnaire) error {
	// 业务规则：并行更新两个数据源，如果任一失败则报错

	mysqlErr := c.mysqlRepo.Update(ctx, q)
	mongoErr := c.documentRepo.UpdateDocument(ctx, q)

	if mysqlErr != nil {
		return fmt.Errorf("failed to update questionnaire basic info: %w", mysqlErr)
	}

	if mongoErr != nil {
		return fmt.Errorf("failed to update questionnaire document: %w", mongoErr)
	}

	return nil
}

// DeleteQuestionnaire 删除问卷（协调多个数据源）
func (c *DataCoordinator) DeleteQuestionnaire(ctx context.Context, id questionnaire.QuestionnaireID) error {
	// 业务规则：先删除文档，再删除基础信息

	var errors []error

	// 1. 删除 MongoDB 文档
	if err := c.documentRepo.RemoveDocument(ctx, id); err != nil {
		errors = append(errors, fmt.Errorf("failed to remove document: %w", err))
	}

	// 2. 删除 MySQL 记录
	if err := c.mysqlRepo.Remove(ctx, id); err != nil {
		errors = append(errors, fmt.Errorf("failed to remove basic info: %w", err))
	}

	if len(errors) > 0 {
		return fmt.Errorf("delete questionnaire failed: %v", errors)
	}

	return nil
}

// SearchQuestionnaires 搜索问卷（应用层业务逻辑）
func (c *DataCoordinator) SearchQuestionnaires(ctx context.Context, query storage.QueryOptions) (*storage.QuestionnaireQueryResult, error) {
	// 应用层决定搜索策略

	// 基础搜索使用 MySQL
	result, err := c.mysqlRepo.FindQuestionnaires(ctx, query)
	if err != nil {
		return nil, err
	}

	// 如果有文档内容搜索需求，可以在这里添加MongoDB搜索
	// 这是应用层的业务逻辑决策

	return result, nil
}

// CheckDataConsistency 检查数据一致性（应用层业务规则）
func (c *DataCoordinator) CheckDataConsistency(ctx context.Context, id questionnaire.QuestionnaireID) error {
	// 检查MySQL和MongoDB中的数据是否一致

	mysqlExists, err := c.mysqlRepo.ExistsByID(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to check MySQL existence: %w", err)
	}

	// 检查MongoDB文档是否存在（通过尝试获取文档）
	_, err = c.documentRepo.GetDocument(ctx, id)
	mongoExists := err == nil
	if err != nil && err != questionnaire.ErrQuestionnaireNotFound {
		return fmt.Errorf("failed to check MongoDB existence: %w", err)
	}

	if mysqlExists && !mongoExists {
		return fmt.Errorf("data inconsistency: MySQL record exists but MongoDB document missing")
	}

	if !mysqlExists && mongoExists {
		return fmt.Errorf("data inconsistency: MongoDB document exists but MySQL record missing")
	}

	return nil
}

// mergeQuestionnaireData 合并问卷数据（应用层业务逻辑）
func (c *DataCoordinator) mergeQuestionnaireData(baseQ *questionnaire.Questionnaire, document interface{}) (*questionnaire.Questionnaire, error) {
	// TODO: 实现具体的数据合并逻辑
	// 这里是应用层决定如何将MySQL的基础信息和MongoDB的文档结构合并

	// 示例：根据文档内容更新问卷的某些字段
	// mergedQ := questionnaire.NewQuestionnaire(...)
	// mergedQ.UpdateFromDocument(document)

	return baseQ, nil
}

// GetBasicQuestionnaireInfo 获取基础问卷信息（直接委托给MySQL）
func (c *DataCoordinator) GetBasicQuestionnaireInfo(ctx context.Context, id questionnaire.QuestionnaireID) (*questionnaire.Questionnaire, error) {
	return c.mysqlRepo.FindByID(ctx, id)
}

// GetQuestionnaireDocument 获取问卷文档（直接委托给MongoDB）
func (c *DataCoordinator) GetQuestionnaireDocument(ctx context.Context, id questionnaire.QuestionnaireID) (interface{}, error) {
	return c.documentRepo.GetDocument(ctx, id)
}
