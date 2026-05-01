package scale

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	domainScale "github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// UpdateBasicInfo 更新基本信息
func (s *lifecycleService) UpdateBasicInfo(ctx context.Context, dto UpdateScaleBasicInfoDTO) (*ScaleResult, error) {
	if dto.Code == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "量表编码不能为空")
	}
	if dto.Title == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "量表标题不能为空")
	}

	m, err := s.getScaleAndValidateEditable(ctx, dto.Code)
	if err != nil {
		return nil, err
	}

	classification := scaleClassificationFromDTO(dto.Category, dto.Stages, dto.ApplicableAges, dto.Reporters, dto.Tags)
	if err := s.baseInfo.UpdateAllWithClassification(m, dto.Title, dto.Description, classification.category, classification.stages, classification.applicableAges, classification.reporters, classification.tags); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrInvalidArgument, "更新基本信息失败")
	}

	if err := s.repo.Update(ctx, m); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存量表基本信息失败")
	}

	s.publishScaleChanged(ctx, m, domainScale.ChangeActionUpdated)
	s.refreshListCache(ctx)

	return toScaleResult(m), nil
}

// UpdateQuestionnaire 更新关联的问卷
func (s *lifecycleService) UpdateQuestionnaire(ctx context.Context, dto UpdateScaleQuestionnaireDTO) (*ScaleResult, error) {
	if dto.Code == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "量表编码不能为空")
	}
	if dto.QuestionnaireCode == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "问卷编码不能为空")
	}
	if dto.QuestionnaireVersion == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "问卷版本不能为空")
	}

	m, err := s.getScaleAndValidateEditable(ctx, dto.Code)
	if err != nil {
		return nil, err
	}

	if err := s.resolveQuestionnaireBinding().validate(ctx, dto.QuestionnaireCode, dto.QuestionnaireVersion, m.GetCode().String()); err != nil {
		return nil, err
	}
	if err := s.baseInfo.UpdateQuestionnaire(m, meta.NewCode(dto.QuestionnaireCode), dto.QuestionnaireVersion); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrInvalidArgument, "更新关联问卷失败")
	}

	if err := s.repo.Update(ctx, m); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存量表关联问卷失败")
	}

	s.publishScaleChanged(ctx, m, domainScale.ChangeActionUpdated)
	s.refreshListCache(ctx)

	return toScaleResult(m), nil
}
