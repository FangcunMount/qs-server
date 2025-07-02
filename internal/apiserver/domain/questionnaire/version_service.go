package questionnaire

import (
	"github.com/yshujie/questionnaire-scale/internal/pkg/code"
	"github.com/yshujie/questionnaire-scale/pkg/errors"
)

// VersionService 版本服务
type VersionService struct{}

// Publish 发布问卷
func (VersionService) Publish(q *Questionnaire) error {
	if len(q.questions) == 0 {
		return errors.WithCode(code.ErrQuestionnaireQuestionInvalid, "发布前必须至少包含一个题目")
	}
	if q.status != STATUS_DRAFT {
		return errors.WithCode(code.ErrQuestionnaireStatusInvalid, "只有草稿状态才能发布")
	}
	q.status = STATUS_PUBLISHED
	return nil
}

// Archive 归档问卷
func (VersionService) Archive(q *Questionnaire) error {
	if q.status != STATUS_PUBLISHED {
		return errors.WithCode(code.ErrQuestionnaireStatusInvalid, "只有发布状态才能归档")
	}
	q.status = STATUS_ARCHIVED
	return nil
}

// Clone 克隆问卷
func (VersionService) Clone(q *Questionnaire) *Questionnaire {
	copy := *q
	copy.id = QuestionnaireID{} // 让 repo 生成新 ID
	copy.status = STATUS_DRAFT
	copy.version = copy.version.Increment()
	return &copy
}
