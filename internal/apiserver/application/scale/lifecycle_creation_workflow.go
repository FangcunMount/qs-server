package scale

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	domainScale "github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// Create 创建量表
func (s *lifecycleService) Create(ctx context.Context, dto CreateScaleDTO) (*ScaleResult, error) {
	if dto.Title == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "量表标题不能为空")
	}

	code, err := s.generateScaleCode(dto.Code)
	if err != nil {
		return nil, err
	}

	classification := scaleClassificationFromDTO(dto.Category, dto.Stages, dto.ApplicableAges, dto.Reporters, dto.Tags)

	if dto.QuestionnaireCode != "" {
		if err := s.resolveQuestionnaireBinding().validate(ctx, dto.QuestionnaireCode, dto.QuestionnaireVersion, code.String()); err != nil {
			return nil, err
		}
	}

	m, err := domainScale.NewMedicalScale(
		code,
		dto.Title,
		domainScale.WithDescription(dto.Description),
		domainScale.WithCategory(classification.category),
		domainScale.WithStages(classification.stages),
		domainScale.WithApplicableAges(classification.applicableAges),
		domainScale.WithReporters(classification.reporters),
		domainScale.WithTags(classification.tags),
		domainScale.WithQuestionnaire(meta.NewCode(dto.QuestionnaireCode), dto.QuestionnaireVersion),
		domainScale.WithStatus(domainScale.StatusDraft),
	)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrInvalidArgument, "创建量表失败")
	}

	if err := s.repo.Create(ctx, m); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存量表失败")
	}

	s.refreshListCache(ctx)

	return toScaleResult(m), nil
}
