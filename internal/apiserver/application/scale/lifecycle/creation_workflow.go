package lifecycle

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/scale/shared"
	domscale "github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// Create 创建量表
func (s *lifecycleService) Create(ctx context.Context, dto shared.CreateScaleDTO) (*shared.ScaleResult, error) {
	if dto.Title == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "量表标题不能为空")
	}

	code, err := s.generateScaleCode(dto.Code)
	if err != nil {
		return nil, err
	}

	classification := shared.ClassificationFromDTO(dto.Category, dto.Stages, dto.ApplicableAges, dto.Reporters, dto.Tags)

	if dto.QuestionnaireCode != "" {
		if err := s.resolveQuestionnaireBinding().validate(ctx, dto.QuestionnaireCode, dto.QuestionnaireVersion, code.String()); err != nil {
			return nil, err
		}
	}

	m, err := domscale.NewMedicalScale(
		code,
		dto.Title,
		domscale.WithDescription(dto.Description),
		domscale.WithCategory(classification.Category),
		domscale.WithStages(classification.Stages),
		domscale.WithApplicableAges(classification.ApplicableAges),
		domscale.WithReporters(classification.Reporters),
		domscale.WithTags(classification.Tags),
		domscale.WithQuestionnaire(meta.NewCode(dto.QuestionnaireCode), dto.QuestionnaireVersion),
		domscale.WithStatus(domscale.StatusDraft),
	)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrInvalidArgument, "创建量表失败")
	}

	if err := s.repo.Create(ctx, m); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存量表失败")
	}

	s.refreshListCache(ctx)

	return shared.ToScaleResult(m), nil
}
