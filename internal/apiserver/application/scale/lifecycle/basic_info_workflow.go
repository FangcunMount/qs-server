package lifecycle

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/scale/shared"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// UpdateBasicInfo 更新基本信息
func (s *lifecycleService) UpdateBasicInfo(ctx context.Context, dto shared.UpdateScaleBasicInfoDTO) (*shared.ScaleResult, error) {
	if dto.Code == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "量表编码不能为空")
	}
	if dto.Title == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "量表标题不能为空")
	}

	m, err := s.getScaleByCode(ctx, dto.Code)
	if err != nil {
		return nil, err
	}
	if err := s.ensureHeadEditable(ctx, m); err != nil {
		return nil, err
	}

	classification := shared.ClassificationFromDTO(dto.Category, dto.Stages, dto.ApplicableAges, dto.Reporters, dto.Tags)
	if err := s.baseInfo.UpdateAllWithClassification(m, dto.Title, dto.Description, classification.Category, classification.Stages, classification.ApplicableAges, classification.Reporters, classification.Tags); err != nil {
		return nil, shared.WrapScaleDomainError(err, errorCode.ErrInvalidArgument, "更新基本信息失败")
	}

	if err := s.repo.Update(ctx, m); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存量表基本信息失败")
	}

	s.publishEvents(ctx, m)
	s.refreshListCache(ctx)

	return shared.ToScaleResult(m), nil
}

// UpdateQuestionnaire 更新关联的问卷
func (s *lifecycleService) UpdateQuestionnaire(ctx context.Context, dto shared.UpdateScaleQuestionnaireDTO) (*shared.ScaleResult, error) {
	if dto.Code == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "量表编码不能为空")
	}
	if dto.QuestionnaireCode == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "问卷编码不能为空")
	}
	if dto.QuestionnaireVersion == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "问卷版本不能为空")
	}

	m, err := s.getScaleByCode(ctx, dto.Code)
	if err != nil {
		return nil, err
	}

	if err := s.resolveQuestionnaireBinding().validate(ctx, dto.QuestionnaireCode, dto.QuestionnaireVersion, m.GetCode().String()); err != nil {
		return nil, err
	}
	if err := s.ensureHeadEditable(ctx, m); err != nil {
		return nil, err
	}
	if err := s.baseInfo.UpdateQuestionnaire(m, meta.NewCode(dto.QuestionnaireCode), dto.QuestionnaireVersion); err != nil {
		return nil, shared.WrapScaleDomainError(err, errorCode.ErrInvalidArgument, "更新关联问卷失败")
	}

	if err := s.repo.Update(ctx, m); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存量表关联问卷失败")
	}

	s.publishEvents(ctx, m)
	s.refreshListCache(ctx)

	return shared.ToScaleResult(m), nil
}
