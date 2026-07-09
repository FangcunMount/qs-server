package lifecycle

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/assessmentstore"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/legacyadapter"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
)

// QuestionnaireBindingSyncer updates scale 问卷-version 投影 在之后 问卷 发布。
type QuestionnaireBindingSyncer struct {
	modelRepo modelcatalogport.ModelRepository
}

// NewQuestionnaireBindingSyncer 创建survey-facing scale binding syncer。
func NewQuestionnaireBindingSyncer(modelRepo modelcatalogport.ModelRepository) *QuestionnaireBindingSyncer {
	return &QuestionnaireBindingSyncer{modelRepo: modelRepo}
}

// SyncQuestionnaireVersion synchronizes bound scale 到 newly published 问卷版本。
func (s *QuestionnaireBindingSyncer) SyncQuestionnaireVersion(ctx context.Context, questionnaireCode, version string) error {
	if s == nil {
		return nil
	}
	return syncQuestionnaireVersion(ctx, s.modelRepo, questionnaireCode, version)
}

func syncQuestionnaireVersion(ctx context.Context, repo modelcatalogport.ModelRepository, questionnaireCode, version string) error {
	if repo == nil || questionnaireCode == "" || version == "" {
		return nil
	}

	model, err := assessmentstore.FindScaleByQuestionnaireCode(ctx, repo, questionnaireCode)
	if err != nil {
		if domain.IsNotFound(err) {
			return nil
		}
		return errors.WrapC(err, errorCode.ErrDatabase, "查询关联量表失败")
	}
	if model == nil || model.Binding.QuestionnaireVersion == version {
		return nil
	}
	if !model.IsDraft() {
		return nil
	}

	now := time.Now().UTC()
	if err := model.BindQuestionnaire(domain.QuestionnaireBinding{
		QuestionnaireCode:    questionnaireCode,
		QuestionnaireVersion: version,
	}, now); err != nil {
		return errors.WrapC(err, errorCode.ErrInvalidArgument, "同步量表问卷版本失败")
	}
	if err := legacyadapter.SyncScaleMetadataInModel(model); err != nil {
		return errors.WrapC(err, errorCode.ErrInvalidArgument, "同步量表问卷版本失败")
	}
	if err := assessmentstore.SaveScale(ctx, repo, model); err != nil {
		return errors.WrapC(err, errorCode.ErrDatabase, "保存量表问卷版本失败")
	}
	return nil
}
