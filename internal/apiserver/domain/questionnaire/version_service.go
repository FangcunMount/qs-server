package questionnaire

import (
	"github.com/fangcun-mount/qs-server/internal/pkg/code"
	"github.com/fangcun-mount/qs-server/pkg/errors"
)

// VersionService 版本服务
type VersionService struct{}

// Publish 发布问卷
func (VersionService) Publish(q *Questionnaire) error {
	if len(q.GetQuestions()) == 0 {
		return errors.WithCode(code.ErrQuestionnaireQuestionInvalid, "发布前必须至少包含一个题目")
	}
	if q.GetStatus() != STATUS_DRAFT {
		return errors.WithCode(code.ErrQuestionnaireStatusInvalid, "只有草稿状态才能发布")
	}
	q.status = STATUS_PUBLISHED
	return nil
}

// Unpublish 下架问卷
func (VersionService) Unpublish(q *Questionnaire) error {
	if q.GetStatus() != STATUS_PUBLISHED {
		return errors.WithCode(code.ErrQuestionnaireStatusInvalid, "只有发布状态才能下架")
	}
	q.status = STATUS_DRAFT
	return nil
}

// Archive 归档问卷
func (VersionService) Archive(q *Questionnaire) error {
	if q.GetStatus() != STATUS_PUBLISHED {
		return errors.WithCode(code.ErrQuestionnaireStatusInvalid, "只有发布状态才能归档")
	}
	q.status = STATUS_ARCHIVED
	return nil
}

// Clone 克隆问卷
func (VersionService) Clone(q *Questionnaire) *Questionnaire {
	copy := *q
	copy.status = STATUS_DRAFT
	copy.version = copy.version.Increment()
	return &copy
}
