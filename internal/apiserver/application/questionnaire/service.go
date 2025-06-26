package questionnaire

import (
	"context"
	"fmt"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/application/questionnaire/commands"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/application/questionnaire/dto"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/application/questionnaire/queries"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/application/shared/interfaces"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/ports/storage"
)

// Service 问卷应用服务
// 作为聚合根的统一入口，协调命令和查询处理器
// 使用DataCoordinator来协调多个数据源的操作
type Service struct {
	// 数据协调器（应用层组件）
	dataCoordinator *DataCoordinator
	// 命令处理器
	commandHandlers *commands.CommandHandlers
	// 查询处理器
	queryHandlers *queries.QueryHandlers
	// 直接访问的仓储（用于简单操作）
	questionnaireRepo storage.QuestionnaireRepository
}

// NewService 创建问卷应用服务
// 接受两个独立的存储库，在应用层进行协调
func NewService(
	mysqlRepo storage.QuestionnaireRepository,
	mongoRepo storage.QuestionnaireDocumentRepository,
) *Service {
	// 创建数据协调器
	dataCoordinator := NewDataCoordinator(mysqlRepo, mongoRepo)

	return &Service{
		dataCoordinator:   dataCoordinator,
		commandHandlers:   commands.NewCommandHandlers(mysqlRepo), // 命令处理器使用MySQL仓储
		queryHandlers:     queries.NewQueryHandlers(mysqlRepo),    // 查询处理器使用MySQL仓储
		questionnaireRepo: mysqlRepo,
	}
}

// NewServiceWithSingleRepo 创建单一数据源的问卷应用服务（向后兼容）
func NewServiceWithSingleRepo(questionnaireRepo storage.QuestionnaireRepository) *Service {
	return &Service{
		dataCoordinator:   nil, // 单一数据源时不需要协调器
		commandHandlers:   commands.NewCommandHandlers(questionnaireRepo),
		queryHandlers:     queries.NewQueryHandlers(questionnaireRepo),
		questionnaireRepo: questionnaireRepo,
	}
}

// ServiceName 实现ApplicationService接口
func (s *Service) ServiceName() string {
	return "questionnaire-service"
}

// ExecuteInTransaction 在事务中执行操作
func (s *Service) ExecuteInTransaction(ctx context.Context, fn func(ctx context.Context) error) error {
	// 这里可以实现事务逻辑
	// 简化实现，直接执行函数
	return fn(ctx)
}

// 命令处理方法 - 使用数据协调器处理多数据源操作

// CreateQuestionnaire 创建问卷（使用数据协调器）
func (s *Service) CreateQuestionnaire(ctx context.Context, cmd commands.CreateQuestionnaireCommand) (*dto.QuestionnaireDTO, error) {
	// 如果有数据协调器，使用协调器处理多数据源
	if s.dataCoordinator != nil {
		// 先通过命令处理器创建问卷
		questionnaireDTO, err := s.commandHandlers.CreateQuestionnaire.Handle(ctx, cmd)
		if err != nil {
			return nil, err
		}

		// 转换为领域对象
		// TODO: 实现DTO到领域对象的转换
		// questionnaire := convertDTOToDomain(questionnaireDTO)

		// 使用协调器保存到多个数据源
		// if err := s.dataCoordinator.SaveQuestionnaire(ctx, questionnaire); err != nil {
		//     return nil, err
		// }

		return questionnaireDTO, nil
	}

	// 单一数据源时使用命令处理器
	return s.commandHandlers.CreateQuestionnaire.Handle(ctx, cmd)
}

// UpdateQuestionnaire 更新问卷（使用数据协调器）
func (s *Service) UpdateQuestionnaire(ctx context.Context, cmd commands.UpdateQuestionnaireCommand) (*dto.QuestionnaireDTO, error) {
	if s.dataCoordinator != nil {
		// 使用数据协调器处理多数据源更新
		// TODO: 实现完整的多数据源更新逻辑
		return s.commandHandlers.UpdateQuestionnaire.Handle(ctx, cmd)
	}

	return s.commandHandlers.UpdateQuestionnaire.Handle(ctx, cmd)
}

// PublishQuestionnaire 发布问卷
func (s *Service) PublishQuestionnaire(ctx context.Context, cmd commands.PublishQuestionnaireCommand) error {
	return s.commandHandlers.PublishQuestionnaire.Handle(ctx, cmd)
}

// DeleteQuestionnaire 删除问卷（使用数据协调器）
func (s *Service) DeleteQuestionnaire(ctx context.Context, cmd commands.DeleteQuestionnaireCommand) error {
	if s.dataCoordinator != nil {
		// 将字符串ID转换为QuestionnaireID类型
		questionnaireID := questionnaire.NewQuestionnaireID(cmd.ID)

		// 使用数据协调器删除多数据源
		return s.dataCoordinator.DeleteQuestionnaire(ctx, questionnaireID)
	}

	return s.commandHandlers.DeleteQuestionnaire.Handle(ctx, cmd)
}

// 查询处理方法 - 根据需要使用数据协调器

// GetQuestionnaire 获取问卷（支持完整数据合并）
func (s *Service) GetQuestionnaire(ctx context.Context, query queries.GetQuestionnaireQuery) (*dto.QuestionnaireDTO, error) {
	// 默认使用查询处理器
	return s.queryHandlers.GetQuestionnaire.Handle(ctx, query)
}

// GetCompleteQuestionnaire 获取完整问卷（包含文档结构） - 新增方法
func (s *Service) GetCompleteQuestionnaire(ctx context.Context, questionnaireID string) (*dto.QuestionnaireDTO, error) {
	if s.dataCoordinator != nil {
		// 获取完整问卷（包含文档结构）
		questionnaireIDObj := questionnaire.NewQuestionnaireID(questionnaireID)

		completeQ, err := s.dataCoordinator.GetCompleteQuestionnaire(ctx, questionnaireIDObj)
		if err != nil {
			return nil, err
		}

		// 转换为DTO
		// TODO: 实现领域对象到DTO的转换
		_ = completeQ

		// 暂时使用查询处理器
		query := queries.GetQuestionnaireQuery{ID: &questionnaireID}
		return s.queryHandlers.GetQuestionnaire.Handle(ctx, query)
	}

	// 单一数据源时使用标准方法
	query := queries.GetQuestionnaireQuery{ID: &questionnaireID}
	return s.GetQuestionnaire(ctx, query)
}

// ListQuestionnaires 获取问卷列表
func (s *Service) ListQuestionnaires(ctx context.Context, query queries.ListQuestionnairesQuery) (*dto.QuestionnaireListDTO, error) {
	return s.queryHandlers.ListQuestionnaires.Handle(ctx, query)
}

// SearchQuestionnaires 搜索问卷（支持文档内容搜索）
func (s *Service) SearchQuestionnaires(ctx context.Context, query queries.SearchQuestionnairesQuery) (*dto.QuestionnaireListDTO, error) {
	if s.dataCoordinator != nil {
		// 使用数据协调器进行高级搜索
		// TODO: 实现包含文档内容的搜索
		return s.queryHandlers.SearchQuestionnaires.Handle(ctx, query)
	}

	return s.queryHandlers.SearchQuestionnaires.Handle(ctx, query)
}

// GetQuestionnaireStatistics 获取问卷统计
func (s *Service) GetQuestionnaireStatistics(ctx context.Context, query queries.GetQuestionnaireStatisticsQuery) (*dto.QuestionnaireStatisticsDTO, error) {
	return s.queryHandlers.GetQuestionnaireStatistics.Handle(ctx, query)
}

// 数据一致性相关方法

// CheckQuestionnaireDataConsistency 检查问卷数据一致性
func (s *Service) CheckQuestionnaireDataConsistency(ctx context.Context, questionnaireID string) (map[string]interface{}, error) {
	if s.dataCoordinator == nil {
		return map[string]interface{}{
			"message": "Single data source mode, no consistency check needed",
			"status":  "ok",
		}, nil
	}

	questionnaireIDObj := questionnaire.NewQuestionnaireID(questionnaireID)

	err := s.dataCoordinator.CheckDataConsistency(ctx, questionnaireIDObj)
	if err != nil {
		return map[string]interface{}{
			"questionnaire_id": questionnaireID,
			"status":           "inconsistent",
			"error":            err.Error(),
		}, nil
	}

	return map[string]interface{}{
		"questionnaire_id": questionnaireID,
		"status":           "consistent",
	}, nil
}

// RepairQuestionnaireData 修复问卷数据不一致
func (s *Service) RepairQuestionnaireData(ctx context.Context, questionnaireID string) error {
	if s.dataCoordinator == nil {
		return fmt.Errorf("data repair not available in single data source mode")
	}

	questionnaireIDObj := questionnaire.NewQuestionnaireID(questionnaireID)

	// 尝试从MySQL恢复到MongoDB
	baseQ, err := s.dataCoordinator.GetBasicQuestionnaireInfo(ctx, questionnaireIDObj)
	if err != nil {
		return fmt.Errorf("failed to get basic questionnaire info: %w", err)
	}

	// 重新保存到多数据源
	return s.dataCoordinator.SaveQuestionnaire(ctx, baseQ)
}

// 高级用例方法 - 组合多个操作（保持原有功能）

// CreateAndPublishQuestionnaire 创建并发布问卷
func (s *Service) CreateAndPublishQuestionnaire(ctx context.Context, createCmd commands.CreateQuestionnaireCommand) (*dto.QuestionnaireDTO, error) {
	// 在事务中执行
	var result *dto.QuestionnaireDTO
	err := s.ExecuteInTransaction(ctx, func(ctx context.Context) error {
		// 1. 创建问卷
		questionnaire, err := s.CreateQuestionnaire(ctx, createCmd)
		if err != nil {
			return err
		}

		// 2. 发布问卷
		publishCmd := commands.PublishQuestionnaireCommand{ID: questionnaire.ID}
		if err := s.PublishQuestionnaire(ctx, publishCmd); err != nil {
			return err
		}

		// 3. 重新获取更新后的问卷
		getQuery := queries.GetQuestionnaireQuery{ID: &questionnaire.ID}
		result, err = s.GetQuestionnaire(ctx, getQuery)
		return err
	})

	return result, err
}

// CloneQuestionnaire 克隆问卷
func (s *Service) CloneQuestionnaire(ctx context.Context, sourceID string, newCode, newTitle string, createdBy string) (*dto.QuestionnaireDTO, error) {
	var result *dto.QuestionnaireDTO
	err := s.ExecuteInTransaction(ctx, func(ctx context.Context) error {
		// 1. 获取源问卷（如果需要完整文档结构，使用专用方法）
		var sourceQuestionnaire *dto.QuestionnaireDTO
		var err error

		if s.dataCoordinator != nil {
			// 使用完整问卷获取方法
			sourceQuestionnaire, err = s.GetCompleteQuestionnaire(ctx, sourceID)
		} else {
			// 使用标准查询
			getQuery := queries.GetQuestionnaireQuery{ID: &sourceID}
			sourceQuestionnaire, err = s.GetQuestionnaire(ctx, getQuery)
		}

		if err != nil {
			return err
		}

		// 2. 创建新问卷
		createCmd := commands.CreateQuestionnaireCommand{
			Code:        newCode,
			Title:       newTitle,
			Description: sourceQuestionnaire.Description,
			CreatorID:   createdBy,
		}

		result, err = s.CreateQuestionnaire(ctx, createCmd)
		return err
	})

	return result, err
}

// BulkUpdateQuestionnaireStatus 批量更新问卷状态
func (s *Service) BulkUpdateQuestionnaireStatus(ctx context.Context, questionnaireIDs []string, action string) error {
	return s.ExecuteInTransaction(ctx, func(ctx context.Context) error {
		for _, id := range questionnaireIDs {
			switch action {
			case "publish":
				cmd := commands.PublishQuestionnaireCommand{ID: id}
				if err := s.PublishQuestionnaire(ctx, cmd); err != nil {
					return err
				}
			case "delete":
				cmd := commands.DeleteQuestionnaireCommand{ID: id}
				if err := s.DeleteQuestionnaire(ctx, cmd); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// GetQuestionnairesByCreator 获取指定创建者的问卷列表
func (s *Service) GetQuestionnairesByCreator(ctx context.Context, creatorID string, pagination interfaces.PaginationRequest) (*dto.QuestionnaireListDTO, error) {
	query := queries.ListQuestionnairesQuery{
		PaginationRequest: pagination,
		QuestionnaireFilterDTO: dto.QuestionnaireFilterDTO{
			CreatorID: &creatorID,
		},
	}
	return s.ListQuestionnaires(ctx, query)
}

// GetPublishedQuestionnaires 获取已发布的问卷列表
func (s *Service) GetPublishedQuestionnaires(ctx context.Context, pagination interfaces.PaginationRequest) (*dto.QuestionnaireListDTO, error) {
	status := questionnaire.StatusPublished
	query := queries.ListQuestionnairesQuery{
		PaginationRequest: pagination,
		QuestionnaireFilterDTO: dto.QuestionnaireFilterDTO{
			Status: &status,
		},
	}
	return s.ListQuestionnaires(ctx, query)
}

// SearchQuestionnairesByKeyword 按关键字搜索问卷
func (s *Service) SearchQuestionnairesByKeyword(ctx context.Context, keyword string, pagination interfaces.PaginationRequest) (*dto.QuestionnaireListDTO, error) {
	query := queries.SearchQuestionnairesQuery{
		PaginationRequest: pagination,
		FilterRequest: interfaces.FilterRequest{
			Keyword: &keyword,
		},
		SortingRequest: interfaces.SortingRequest{
			SortBy:    "updated_at",
			SortOrder: "desc",
		},
	}
	return s.SearchQuestionnaires(ctx, query)
}

// ValidateQuestionnaire 验证问卷完整性
func (s *Service) ValidateQuestionnaire(ctx context.Context, questionnaireID string) (map[string]interface{}, error) {
	// 获取问卷
	getQuery := queries.GetQuestionnaireQuery{ID: &questionnaireID}
	questionnaire, err := s.GetQuestionnaire(ctx, getQuery)
	if err != nil {
		return nil, err
	}

	// 执行验证逻辑
	validation := map[string]interface{}{
		"questionnaire_id": questionnaireID,
		"valid":            true,
		"issues":           []string{},
		"question_count":   len(questionnaire.Questions),
		"has_title":        questionnaire.Title != "",
		"has_description":  questionnaire.Description != "",
	}

	var issues []string

	// 检查标题
	if questionnaire.Title == "" {
		issues = append(issues, "Missing title")
		validation["valid"] = false
	}

	// 检查是否有问题
	if len(questionnaire.Questions) == 0 {
		issues = append(issues, "No questions defined")
		validation["valid"] = false
	}

	// 检查问题完整性
	for i, question := range questionnaire.Questions {
		if question.Title == "" {
			issues = append(issues, fmt.Sprintf("Question %d has no title", i+1))
			validation["valid"] = false
		}
		// 可以添加更多验证规则
	}

	// 如果有数据协调器，检查数据一致性
	if s.dataCoordinator != nil {
		consistency, err := s.CheckQuestionnaireDataConsistency(ctx, questionnaireID)
		if err != nil {
			issues = append(issues, fmt.Sprintf("Data consistency check failed: %v", err))
			validation["valid"] = false
		} else if consistency["status"] != "consistent" {
			issues = append(issues, "Data inconsistency detected between MySQL and MongoDB")
			validation["valid"] = false
		}
		validation["data_consistency"] = consistency
	}

	validation["issues"] = issues
	return validation, nil
}
