package lifecycle

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	domscale "github.com/FangcunMount/qs-server/internal/apiserver/domain/authoring/scale"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
)

// QuestionnaireBindingSyncer updates scale questionnaire-version projection after questionnaire publication.
type QuestionnaireBindingSyncer struct {
	repo questionnaireBindingSyncRepository
}

type questionnaireBindingSyncRepository interface {
	FindByQuestionnaireCode(ctx context.Context, questionnaireCode string) (*domscale.MedicalScale, error)
	Update(ctx context.Context, scale *domscale.MedicalScale) error
}

// NewQuestionnaireBindingSyncer creates a survey-facing scale binding syncer.
func NewQuestionnaireBindingSyncer(repo questionnaireBindingSyncRepository) *QuestionnaireBindingSyncer {
	return &QuestionnaireBindingSyncer{repo: repo}
}

// SyncQuestionnaireVersion synchronizes a bound scale to the newly published questionnaire version.
func (s *QuestionnaireBindingSyncer) SyncQuestionnaireVersion(ctx context.Context, questionnaireCode, version string) error {
	if s == nil {
		return nil
	}
	return syncQuestionnaireVersion(ctx, s.repo, questionnaireCode, version)
}

func syncQuestionnaireVersion(ctx context.Context, repo questionnaireBindingSyncRepository, questionnaireCode, version string) error {
	if repo == nil || questionnaireCode == "" || version == "" {
		return nil
	}

	item, err := repo.FindByQuestionnaireCode(ctx, questionnaireCode)
	if err != nil {
		if domscale.IsNotFound(err) {
			return nil
		}
		return errors.WrapC(err, errorCode.ErrDatabase, "查询关联量表失败")
	}
	if item == nil || item.GetQuestionnaireVersion() == version {
		return nil
	}
	if !item.IsDraft() {
		return nil
	}

	baseInfo := domscale.BaseInfo{}
	if err := baseInfo.UpdateQuestionnaire(item, item.GetQuestionnaireCode(), version); err != nil {
		return errors.WrapC(err, errorCode.ErrInvalidArgument, "同步量表问卷版本失败")
	}
	if err := repo.Update(ctx, item); err != nil {
		return errors.WrapC(err, errorCode.ErrDatabase, "保存量表问卷版本失败")
	}
	return nil
}
