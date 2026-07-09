package assessmentstore

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/legacyadapter"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/shared"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
)

// LoadScale loads a draft medical-scale AssessmentModel by code.
func LoadScale(ctx context.Context, repo modelcatalogport.ModelRepository, code string) (*domain.AssessmentModel, error) {
	if code == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "量表编码不能为空")
	}
	if repo == nil {
		return nil, errors.WithCode(errorCode.ErrModuleInitializationFailed, "测评模型仓储未配置")
	}
	model, err := repo.FindByCode(ctx, code)
	if err != nil {
		if domain.IsNotFound(err) {
			return nil, errors.WrapC(err, errorCode.ErrMedicalScaleNotFound, "获取量表失败")
		}
		return nil, err
	}
	if model == nil || model.Kind != domain.KindScale {
		return nil, errors.WithCode(errorCode.ErrMedicalScaleNotFound, "获取量表失败")
	}
	return model, nil
}

// EnsureHeadEditable forks a published scale head into draft when needed.
func EnsureHeadEditable(ctx context.Context, repo modelcatalogport.ModelRepository, model *domain.AssessmentModel) error {
	if model == nil {
		return nil
	}
	if model.IsArchived() {
		return errors.WithCode(errorCode.ErrInvalidArgument, "量表已归档，不能编辑")
	}
	if !model.IsPublished() {
		return nil
	}
	now := time.Now().UTC()
	if err := legacyadapter.ForkAssessmentModelDraftFromPublished(model, now); err != nil {
		return errors.WrapC(err, errorCode.ErrInvalidArgument, "派生草稿量表失败")
	}
	return SaveScale(ctx, repo, model)
}

// SaveScale persists a draft medical-scale AssessmentModel.
func SaveScale(ctx context.Context, repo modelcatalogport.ModelRepository, model *domain.AssessmentModel) error {
	if repo == nil {
		return errors.WithCode(errorCode.ErrModuleInitializationFailed, "测评模型仓储未配置")
	}
	if err := repo.Update(ctx, model); err != nil {
		return errors.WrapC(err, errorCode.ErrDatabase, "保存量表失败")
	}
	return nil
}

// ScaleResult projects a draft AssessmentModel to the legacy scale API shape.
func ScaleResult(model *domain.AssessmentModel) (*shared.ScaleResult, error) {
	result, err := legacyadapter.ScaleResultFromAssessmentModel(model)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrInvalidArgument, "转换量表结果失败")
	}
	return result, nil
}
