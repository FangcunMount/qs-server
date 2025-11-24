package testee_management

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
)

// queryService 受试者查询服务实现
type queryService struct {
	repo testee.Repository
}

// NewQueryService 创建受试者查询服务
func NewQueryService(repo testee.Repository) TesteeQueryApplicationService {
	return &queryService{
		repo: repo,
	}
}

// GetByID 根据ID查询受试者
func (s *queryService) GetByID(ctx context.Context, testeeID uint64) (*TesteeManagementResult, error) {
	t, err := s.repo.FindByID(ctx, testee.ID(testeeID))
	if err != nil {
		return nil, errors.Wrap(err, "failed to find testee")
	}

	return toManagementResult(t), nil
}

// FindByIAMChild 根据 IAM Child ID 查询受试者
func (s *queryService) FindByIAMChild(ctx context.Context, orgID int64, iamChildID int64) (*TesteeManagementResult, error) {
	t, err := s.repo.FindByIAMChild(ctx, orgID, iamChildID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to find testee by iam child id")
	}

	return toManagementResult(t), nil
}

// ListTestees 列出受试者
func (s *queryService) ListTestees(ctx context.Context, dto ListTesteeDTO) (*TesteeListResult, error) {
	// 根据条件选择查询方法
	var testees []*testee.Testee
	var err error

	if dto.KeyFocus != nil && *dto.KeyFocus {
		testees, err = s.repo.ListKeyFocus(ctx, dto.OrgID, dto.Offset, dto.Limit)
	} else if len(dto.Tags) > 0 {
		testees, err = s.repo.ListByTags(ctx, dto.OrgID, dto.Tags, dto.Offset, dto.Limit)
	} else if dto.Name != "" {
		testees, err = s.repo.FindByOrgAndName(ctx, dto.OrgID, dto.Name)
	} else {
		testees, err = s.repo.ListByOrg(ctx, dto.OrgID, dto.Offset, dto.Limit)
	}

	if err != nil {
		return nil, errors.Wrap(err, "failed to list testees")
	}

	// 获取总数
	totalCount, err := s.repo.Count(ctx, dto.OrgID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to count testees")
	}

	// 转换为 DTO
	items := make([]*TesteeManagementResult, len(testees))
	for i, t := range testees {
		items[i] = toManagementResult(t)
	}

	return &TesteeListResult{
		Items:      items,
		TotalCount: totalCount,
		Offset:     dto.Offset,
		Limit:      dto.Limit,
	}, nil
}

// ListKeyFocus 列出重点关注的受试者
func (s *queryService) ListKeyFocus(ctx context.Context, orgID int64, offset, limit int) (*TesteeListResult, error) {
	testees, err := s.repo.ListKeyFocus(ctx, orgID, offset, limit)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list key focus testees")
	}

	// 获取总数
	totalCount, err := s.repo.Count(ctx, orgID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to count testees")
	}

	// 转换为 DTO
	items := make([]*TesteeManagementResult, len(testees))
	for i, t := range testees {
		items[i] = toManagementResult(t)
	}

	return &TesteeListResult{
		Items:      items,
		TotalCount: totalCount,
		Offset:     offset,
		Limit:      limit,
	}, nil
}

// toManagementResult 将领域对象转换为管理 DTO
func toManagementResult(t *testee.Testee) *TesteeManagementResult {
	if t == nil {
		return nil
	}

	result := &TesteeManagementResult{
		ID:         uint64(t.ID()),
		OrgID:      t.OrgID(),
		IAMUserID:  t.IAMUserID(),
		IAMChildID: t.IAMChildID(),
		Name:       t.Name(),
		Gender:     int8(t.Gender()),
		Birthday:   t.Birthday(),
		Tags:       t.Tags(),
		Source:     t.Source(),
		IsKeyFocus: t.IsKeyFocus(),
	}

	// 计算年龄
	if t.Birthday() != nil {
		result.Age = calculateAge(*t.Birthday())
	}

	// 映射测评统计
	if stats := t.AssessmentStats(); stats != nil {
		lastAt := stats.LastAssessmentAt()
		result.LastAssessmentAt = &lastAt
		result.TotalAssessments = stats.TotalCount()
		result.LastRiskLevel = stats.LastRiskLevel()
	}

	return result
}

// calculateAge 计算年龄
func calculateAge(birthday time.Time) int {
	now := time.Now()
	age := now.Year() - birthday.Year()

	// 如果今年的生日还没到，年龄减1
	if now.Month() < birthday.Month() || (now.Month() == birthday.Month() && now.Day() < birthday.Day()) {
		age--
	}

	return age
}
