package shared

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// compositeService 是聚合服务的实现
// 它组合多个细粒度的应用服务，提供统一的接口
type compositeService struct {
	registrationService TesteeRegistrationApplicationService
	profileService      TesteeProfileApplicationService
	tagService          TesteeTagApplicationService
	queryService        TesteeQueryApplicationService
}

// NewCompositeService 创建聚合服务
func NewCompositeService(
	registrationService TesteeRegistrationApplicationService,
	profileService TesteeProfileApplicationService,
	tagService TesteeTagApplicationService,
	queryService TesteeQueryApplicationService,
) Service {
	return &compositeService{
		registrationService: registrationService,
		profileService:      profileService,
		tagService:          tagService,
		queryService:        queryService,
	}
}

// Create 创建受试者
func (s *compositeService) Create(ctx context.Context, dto CreateTesteeDTO) (*CompositeTesteeResult, error) {
	// 转换为注册服务的 DTO
	regDTO := RegisterTesteeDTO{
		OrgID:     dto.OrgID,
		ProfileID: dto.ProfileID,
		Name:      dto.Name,
		Gender:    dto.Gender,
		Birthday:  dto.Birthday,
		Source:    dto.Source,
	}

	result, err := s.registrationService.Register(ctx, regDTO)
	if err != nil {
		return nil, err
	}

	testeeID := uint64(result.ID)

	// 添加标签
	for _, tag := range dto.Tags {
		if err := s.tagService.AddTag(ctx, testeeID, tag); err != nil {
			return nil, errors.Wrap(err, "failed to add tag")
		}
	}

	// 如果需要标记为重点关注
	if dto.IsKeyFocus {
		if err := s.tagService.MarkAsKeyFocus(ctx, testeeID); err != nil {
			return nil, errors.Wrap(err, "failed to mark as key focus")
		}
	}

	// 重新查询完整信息
	return s.GetByID(ctx, testeeID)
}

// GetByID 获取受试者详情
func (s *compositeService) GetByID(ctx context.Context, testeeID uint64) (*CompositeTesteeResult, error) {
	result, err := s.queryService.GetByID(ctx, testeeID)
	if err != nil {
		return nil, err
	}

	return toCompositeResult(result), nil
}

// Update 更新受试者
func (s *compositeService) Update(ctx context.Context, testeeID uint64, dto UpdateTesteeDTO) (*CompositeTesteeResult, error) {
	// 更新基本信息
	if dto.Name != nil || dto.Gender != nil || dto.Birthday != nil {
		profileDTO := UpdateTesteeProfileDTO{
			TesteeID: testeeID,
		}
		if dto.Name != nil {
			profileDTO.Name = *dto.Name
		}
		if dto.Gender != nil {
			profileDTO.Gender = *dto.Gender
		}
		if dto.Birthday != nil {
			profileDTO.Birthday = dto.Birthday
		}

		if err := s.profileService.UpdateBasicInfo(ctx, profileDTO); err != nil {
			return nil, errors.Wrap(err, "failed to update basic info")
		}
	}

	// 更新标签（这里简化处理，实际可能需要对比差异）
	// TODO: 实现标签的增量更新逻辑

	// 更新重点关注状态
	if dto.IsKeyFocus != nil {
		if *dto.IsKeyFocus {
			if err := s.tagService.MarkAsKeyFocus(ctx, testeeID); err != nil {
				return nil, errors.Wrap(err, "failed to mark as key focus")
			}
		} else {
			if err := s.tagService.UnmarkKeyFocus(ctx, testeeID); err != nil {
				return nil, errors.Wrap(err, "failed to unmark key focus")
			}
		}
	}

	// 返回更新后的完整信息
	return s.GetByID(ctx, testeeID)
}

// Delete 删除受试者
func (s *compositeService) Delete(ctx context.Context, testeeID uint64) error {
	// TODO: 实现删除逻辑，当前应用服务接口中没有 Delete 方法
	// 可以通过直接调用 repository 或者添加新的应用服务接口
	return errors.WithCode(code.ErrInternalServerError, "delete testee not implemented yet")
}

// FindByName 根据姓名查找受试者
func (s *compositeService) FindByName(ctx context.Context, orgID int64, name string) ([]*CompositeTesteeResult, error) {
	// 使用 ListTestees 方法进行模糊搜索
	listDTO := ListTesteeDTO{
		OrgID:  orgID,
		Name:   name,
		Offset: 0,
		Limit:  1000, // 名字搜索通常结果不会太多
	}

	listResult, err := s.queryService.ListTestees(ctx, listDTO)
	if err != nil {
		return nil, err
	}

	results := make([]*CompositeTesteeResult, 0, len(listResult.Items))
	for _, item := range listResult.Items {
		results = append(results, toCompositeResult(item))
	}

	return results, nil
}

// ListByTags 根据标签列表查询
func (s *compositeService) ListByTags(ctx context.Context, orgID int64, tags []string, offset, limit int) ([]*CompositeTesteeResult, error) {
	listDTO := ListTesteeDTO{
		OrgID:  orgID,
		Tags:   tags,
		Offset: offset,
		Limit:  limit,
	}

	listResult, err := s.queryService.ListTestees(ctx, listDTO)
	if err != nil {
		return nil, err
	}

	results := make([]*CompositeTesteeResult, 0, len(listResult.Items))
	for _, item := range listResult.Items {
		results = append(results, toCompositeResult(item))
	}

	return results, nil
}

// ListKeyFocus 查询重点关注的受试者
func (s *compositeService) ListKeyFocus(ctx context.Context, orgID int64, offset, limit int) ([]*CompositeTesteeResult, error) {
	listResult, err := s.queryService.ListKeyFocus(ctx, orgID, offset, limit)
	if err != nil {
		return nil, err
	}

	results := make([]*CompositeTesteeResult, 0, len(listResult.Items))
	for _, item := range listResult.Items {
		results = append(results, toCompositeResult(item))
	}

	return results, nil
}

// ListByOrg 查询机构下所有受试者
func (s *compositeService) ListByOrg(ctx context.Context, orgID int64, offset, limit int) ([]*CompositeTesteeResult, error) {
	listDTO := ListTesteeDTO{
		OrgID:  orgID,
		Offset: offset,
		Limit:  limit,
	}

	listResult, err := s.queryService.ListTestees(ctx, listDTO)
	if err != nil {
		return nil, err
	}

	results := make([]*CompositeTesteeResult, 0, len(listResult.Items))
	for _, item := range listResult.Items {
		results = append(results, toCompositeResult(item))
	}

	return results, nil
}

// CountByOrg 统计机构下的受试者数量
func (s *compositeService) CountByOrg(ctx context.Context, orgID int64) (int64, error) {
	listDTO := ListTesteeDTO{
		OrgID:  orgID,
		Offset: 0,
		Limit:  1, // 只需要总数
	}

	listResult, err := s.queryService.ListTestees(ctx, listDTO)
	if err != nil {
		return 0, err
	}

	return listResult.TotalCount, nil
}

// FindByProfileID 根据用户档案 ID 查找受试者
func (s *compositeService) FindByProfileID(ctx context.Context, orgID int64, profileID uint64) (*CompositeTesteeResult, error) {
	result, err := s.queryService.FindByProfile(ctx, orgID, profileID)
	if err != nil {
		return nil, err
	}

	return toCompositeResult(result), nil
}

// toCompositeResult 转换为 CompositeTesteeResult
func toCompositeResult(src *TesteeManagementResult) *CompositeTesteeResult {
	result := &CompositeTesteeResult{
		ID:         src.ID,
		OrgID:      src.OrgID,
		ProfileID:  src.ProfileID,
		Name:       src.Name,
		Gender:     src.Gender,
		Birthday:   src.Birthday,
		Age:        src.Age,
		Tags:       src.Tags,
		Source:     src.Source,
		IsKeyFocus: src.IsKeyFocus,
	}

	// 转换测评统计信息
	if src.LastAssessmentAt != nil || src.TotalAssessments > 0 {
		result.AssessmentStats = &AssessmentStatsResult{
			LastAssessmentAt: src.LastAssessmentAt,
			TotalAssessments: src.TotalAssessments,
			LastRiskLevel:    src.LastRiskLevel,
		}
	}

	return result
}
