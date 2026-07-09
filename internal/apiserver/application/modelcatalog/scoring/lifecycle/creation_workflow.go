package lifecycle

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/assessmentstore"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/legacyadapter"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/shared"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
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

	if dto.QuestionnaireCode != "" {
		if err := s.resolveQuestionnaireBinding().validate(ctx, dto.QuestionnaireCode, dto.QuestionnaireVersion, code.String()); err != nil {
			return nil, err
		}
	}
	dto.Code = code.String()

	model, err := legacyadapter.AssessmentModelFromCreateDTO(dto, time.Now().UTC())
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrInvalidArgument, "创建量表失败")
	}
	if err := s.modelRepo.Create(ctx, model); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存量表失败")
	}
	s.refreshListCache(ctx)
	return assessmentstore.ScaleResult(model)
}
