package scale

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// lifecycleService 量表生命周期服务实现
// 行为者：量表设计者/管理员
type lifecycleService struct {
	repo      scale.Repository
	lifecycle scale.Lifecycle
	baseInfo  scale.BaseInfo
}

// NewLifecycleService 创建量表生命周期服务
func NewLifecycleService(repo scale.Repository) ScaleLifecycleService {
	return &lifecycleService{
		repo:      repo,
		lifecycle: scale.NewLifecycle(),
		baseInfo:  scale.BaseInfo{},
	}
}

// Create 创建量表
func (s *lifecycleService) Create(ctx context.Context, dto CreateScaleDTO) (*ScaleResult, error) {
	// 1. 验证输入参数
	if dto.Title == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "量表标题不能为空")
	}

	// 2. 生成量表编码
	code, err := meta.GenerateCode()
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrInvalidArgument, "生成量表编码失败")
	}

	// 3. 创建量表领域模型
	m, err := scale.NewMedicalScale(
		code,
		dto.Title,
		scale.WithDescription(dto.Description),
		scale.WithQuestionnaire(meta.NewCode(dto.QuestionnaireCode), dto.QuestionnaireVersion),
		scale.WithStatus(scale.StatusDraft),
	)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrInvalidArgument, "创建量表失败")
	}

	// 4. 持久化
	if err := s.repo.Create(ctx, m); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存量表失败")
	}

	return toScaleResult(m), nil
}

// UpdateBasicInfo 更新基本信息
func (s *lifecycleService) UpdateBasicInfo(ctx context.Context, dto UpdateScaleBasicInfoDTO) (*ScaleResult, error) {
	// 1. 验证输入参数
	if dto.Code == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "量表编码不能为空")
	}
	if dto.Title == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "量表标题不能为空")
	}

	// 2. 获取现有量表
	m, err := s.repo.FindByCode(ctx, dto.Code)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrMedicalScaleNotFound, "获取量表失败")
	}

	// 3. 判断量表状态
	if m.IsArchived() {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "量表已归档，不能编辑")
	}

	// 4. 更新基本信息
	if err := s.baseInfo.UpdateAll(m, dto.Title, dto.Description); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrInvalidArgument, "更新基本信息失败")
	}

	// 5. 持久化
	if err := s.repo.Update(ctx, m); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存量表基本信息失败")
	}

	return toScaleResult(m), nil
}

// UpdateQuestionnaire 更新关联的问卷
func (s *lifecycleService) UpdateQuestionnaire(ctx context.Context, dto UpdateScaleQuestionnaireDTO) (*ScaleResult, error) {
	// 1. 验证输入参数
	if dto.Code == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "量表编码不能为空")
	}
	if dto.QuestionnaireCode == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "问卷编码不能为空")
	}
	if dto.QuestionnaireVersion == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "问卷版本不能为空")
	}

	// 2. 获取现有量表
	m, err := s.repo.FindByCode(ctx, dto.Code)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrMedicalScaleNotFound, "获取量表失败")
	}

	// 3. 判断量表状态
	if m.IsArchived() {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "量表已归档，不能编辑")
	}

	// 4. 更新关联的问卷
	if err := s.baseInfo.UpdateQuestionnaire(m, meta.NewCode(dto.QuestionnaireCode), dto.QuestionnaireVersion); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrInvalidArgument, "更新关联问卷失败")
	}

	// 5. 持久化
	if err := s.repo.Update(ctx, m); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存量表关联问卷失败")
	}

	return toScaleResult(m), nil
}

// Publish 发布量表
func (s *lifecycleService) Publish(ctx context.Context, code string) (*ScaleResult, error) {
	// 1. 验证输入参数
	if code == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "量表编码不能为空")
	}

	// 2. 获取量表
	m, err := s.repo.FindByCode(ctx, code)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrMedicalScaleNotFound, "获取量表失败")
	}

	// 3. 调用生命周期服务发布量表（包含验证逻辑）
	if err := s.lifecycle.Publish(ctx, m); err != nil {
		return nil, err
	}

	// 4. 持久化
	if err := s.repo.Update(ctx, m); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存量表状态失败")
	}

	return toScaleResult(m), nil
}

// Unpublish 下架量表
func (s *lifecycleService) Unpublish(ctx context.Context, code string) (*ScaleResult, error) {
	// 1. 验证输入参数
	if code == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "量表编码不能为空")
	}

	// 2. 获取量表
	m, err := s.repo.FindByCode(ctx, code)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrMedicalScaleNotFound, "获取量表失败")
	}

	// 3. 调用生命周期服务下架量表
	if err := s.lifecycle.Unpublish(ctx, m); err != nil {
		return nil, err
	}

	// 4. 持久化
	if err := s.repo.Update(ctx, m); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存量表状态失败")
	}

	return toScaleResult(m), nil
}

// Archive 归档量表
func (s *lifecycleService) Archive(ctx context.Context, code string) (*ScaleResult, error) {
	// 1. 验证输入参数
	if code == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "量表编码不能为空")
	}

	// 2. 获取量表
	m, err := s.repo.FindByCode(ctx, code)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrMedicalScaleNotFound, "获取量表失败")
	}

	// 3. 调用生命周期服务归档量表
	if err := s.lifecycle.Archive(ctx, m); err != nil {
		return nil, err
	}

	// 4. 持久化
	if err := s.repo.Update(ctx, m); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存量表状态失败")
	}

	return toScaleResult(m), nil
}

// Delete 删除量表
func (s *lifecycleService) Delete(ctx context.Context, code string) error {
	// 1. 验证输入参数
	if code == "" {
		return errors.WithCode(errorCode.ErrInvalidArgument, "量表编码不能为空")
	}

	// 2. 获取量表
	m, err := s.repo.FindByCode(ctx, code)
	if err != nil {
		return errors.WrapC(err, errorCode.ErrMedicalScaleNotFound, "获取量表失败")
	}

	// 3. 只能删除草稿状态的量表
	if !m.IsDraft() {
		return errors.WithCode(errorCode.ErrInvalidArgument, "只能删除草稿状态的量表")
	}

	// 4. 删除量表
	if err := s.repo.Remove(ctx, code); err != nil {
		return errors.WrapC(err, errorCode.ErrDatabase, "删除量表失败")
	}

	return nil
}
