package scale

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	domainScale "github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
)

// QuestionnaireBindingSyncer updates scale questionnaire-version projection after questionnaire publication.
type QuestionnaireBindingSyncer struct {
	repo domainScale.Repository
}

// NewQuestionnaireBindingSyncer creates a survey-facing scale binding syncer.
func NewQuestionnaireBindingSyncer(repo domainScale.Repository) *QuestionnaireBindingSyncer {
	return &QuestionnaireBindingSyncer{repo: repo}
}

// LazyQuestionnaireBindingSyncer resolves the scale repository when the publish use case runs.
type LazyQuestionnaireBindingSyncer struct {
	repo func() domainScale.Repository
}

// NewLazyQuestionnaireBindingSyncer creates a survey-facing syncer without post-construction module mutation.
func NewLazyQuestionnaireBindingSyncer(repo func() domainScale.Repository) *LazyQuestionnaireBindingSyncer {
	return &LazyQuestionnaireBindingSyncer{repo: repo}
}

// SyncQuestionnaireVersion synchronizes a bound scale to the newly published questionnaire version.
func (s *QuestionnaireBindingSyncer) SyncQuestionnaireVersion(ctx context.Context, questionnaireCode, version string) error {
	if s == nil {
		return nil
	}
	return syncQuestionnaireVersion(ctx, s.repo, questionnaireCode, version)
}

// SyncQuestionnaireVersion synchronizes a bound scale to the newly published questionnaire version.
func (s *LazyQuestionnaireBindingSyncer) SyncQuestionnaireVersion(ctx context.Context, questionnaireCode, version string) error {
	if s == nil || s.repo == nil {
		return nil
	}
	return syncQuestionnaireVersion(ctx, s.repo(), questionnaireCode, version)
}

func syncQuestionnaireVersion(ctx context.Context, repo domainScale.Repository, questionnaireCode, version string) error {
	if repo == nil || questionnaireCode == "" || version == "" {
		return nil
	}

	item, err := repo.FindByQuestionnaireCode(ctx, questionnaireCode)
	if err != nil {
		if domainScale.IsNotFound(err) {
			return nil
		}
		return errors.WrapC(err, errorCode.ErrDatabase, "查询关联量表失败")
	}
	if item == nil || item.GetQuestionnaireVersion() == version {
		return nil
	}

	baseInfo := domainScale.BaseInfo{}
	if err := baseInfo.UpdateQuestionnaire(item, item.GetQuestionnaireCode(), version); err != nil {
		return errors.WrapC(err, errorCode.ErrInvalidArgument, "同步量表问卷版本失败")
	}
	if err := repo.Update(ctx, item); err != nil {
		return errors.WrapC(err, errorCode.ErrDatabase, "保存量表问卷版本失败")
	}
	return nil
}
