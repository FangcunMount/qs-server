package lifecycle

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/legacyadapter"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/shared"
	scaledefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scoring/definition"
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
	dto.Code = code.String()

	if s.modelRepo != nil {
		scale, err := legacyadapter.MedicalScaleFromCreateDTO(dto)
		if err != nil {
			return nil, errors.WrapC(err, errorCode.ErrInvalidArgument, "创建量表失败")
		}
		model, err := legacyadapter.AssessmentModelFromMedicalScale(scale, time.Now().UTC())
		if err != nil {
			return nil, errors.WrapC(err, errorCode.ErrInvalidArgument, "创建量表失败")
		}
		if err := s.modelRepo.Create(ctx, model); err != nil {
			return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存量表失败")
		}
		s.refreshListCache(ctx)
		return shared.ToScaleResult(scale), nil
	}

	m, err := scaledefinition.NewMedicalScale(
		code,
		dto.Title,
		scaledefinition.WithDescription(dto.Description),
		scaledefinition.WithCategory(classification.Category),
		scaledefinition.WithStages(classification.Stages),
		scaledefinition.WithApplicableAges(classification.ApplicableAges),
		scaledefinition.WithReporters(classification.Reporters),
		scaledefinition.WithTags(classification.Tags),
		scaledefinition.WithQuestionnaire(meta.NewCode(dto.QuestionnaireCode), dto.QuestionnaireVersion),
		scaledefinition.WithStatus(scaledefinition.StatusDraft),
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
