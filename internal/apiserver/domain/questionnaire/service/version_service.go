package service

import (
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire"
	"github.com/yshujie/questionnaire-scale/internal/pkg/code"
	"github.com/yshujie/questionnaire-scale/pkg/errors"
)

// VersionService 版本服务
type VersionService struct{}

// Publish 发布问卷
func (VersionService) Publish(q *questionnaire.Questionnaire) error {
	if len(q.GetQuestions()) == 0 {
		return errors.WithCode(code.ErrQuestionnaireQuestionInvalid, "发布前必须至少包含一个题目")
	}
	if q.GetStatus() != questionnaire.STATUS_DRAFT {
		return errors.WithCode(code.ErrQuestionnaireStatusInvalid, "只有草稿状态才能发布")
	}
	q.SetStatus(questionnaire.STATUS_PUBLISHED)
	return nil
}

// Archive 归档问卷
func (VersionService) Archive(q *questionnaire.Questionnaire) error {
	if q.GetStatus() != questionnaire.STATUS_PUBLISHED {
		return errors.WithCode(code.ErrQuestionnaireStatusInvalid, "只有发布状态才能归档")
	}
	q.SetStatus(questionnaire.STATUS_ARCHIVED)
	return nil
}

// Clone 克隆问卷
func (VersionService) Clone(q *questionnaire.Questionnaire) *questionnaire.Questionnaire {
	copy := *q
	copy.SetStatus(questionnaire.STATUS_DRAFT)
	copy.SetVersion(copy.GetVersion().Increment())
	return &copy
}
