package lifecycle

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/questionnairecatalog"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
)

func (s *lifecycleService) ensureBoundQuestionnairePublished(ctx context.Context, scaleCode string, model *domain.AssessmentModel) error {
	if model == nil || model.Binding.QuestionnaireCode == "" {
		return nil
	}

	questionnaireCode := model.Binding.QuestionnaireCode
	if err := s.resolveQuestionnaireBinding().validate(ctx, questionnaireCode, model.Binding.QuestionnaireVersion, scaleCode); err != nil {
		return err
	}
	if s.questionnaireCatalog == nil {
		return errors.WithCode(errorCode.ErrQuestionnaireNotFound, "关联的问卷不存在")
	}

	head, err := s.questionnaireCatalog.FindQuestionnaire(ctx, questionnaireCode)
	if err != nil {
		return errors.WrapC(err, errorCode.ErrQuestionnaireNotFound, "获取关联问卷失败")
	}
	if head == nil {
		return errors.WithCode(errorCode.ErrQuestionnaireNotFound, "关联的问卷不存在")
	}

	published, err := s.questionnaireCatalog.FindPublishedQuestionnaire(ctx, questionnaireCode)
	if err != nil {
		return errors.WrapC(err, errorCode.ErrQuestionnaireNotFound, "获取已发布关联问卷失败")
	}

	targetVersion := model.Binding.QuestionnaireVersion
	if shouldPublishQuestionnaire(head.Status, head.Version, publishedVersion(published)) {
		if s.questionnairePublisher == nil {
			return errors.WithCode(errorCode.ErrModuleInitializationFailed, "量表发布缺少问卷发布服务")
		}
		publishedVersion, err := s.questionnairePublisher.PublishQuestionnaire(ctx, questionnaireCode)
		if err != nil {
			return errors.WrapC(err, errorCode.ErrInvalidArgument, "发布关联问卷失败")
		}
		targetVersion = publishedVersion
	} else if published != nil {
		targetVersion = published.Version
	} else if targetVersion == "" {
		targetVersion = head.Version
	}

	if targetVersion == "" {
		return errors.WithCode(errorCode.ErrQuestionnaireNotFound, "关联问卷版本不存在")
	}
	if model.Binding.QuestionnaireVersion == targetVersion {
		return nil
	}
	return model.BindQuestionnaire(domain.QuestionnaireBinding{
		QuestionnaireCode:    questionnaireCode,
		QuestionnaireVersion: targetVersion,
	}, time.Now().UTC())
}

func shouldPublishQuestionnaire(headStatus, headVersion, activePublishedVersion string) bool {
	if headStatus == "draft" {
		return true
	}
	if activePublishedVersion == "" && headStatus != "published" {
		return true
	}
	return headVersion != "" && activePublishedVersion != "" && headVersion != activePublishedVersion && headStatus != "published"
}

func publishedVersion(item *questionnairecatalog.Item) string {
	if item == nil {
		return ""
	}
	return item.Version
}
