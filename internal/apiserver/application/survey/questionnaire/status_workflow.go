package questionnaire

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	domainQuestionnaire "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
)

// Unpublish 下架问卷
func (s *lifecycleService) Unpublish(ctx context.Context, code string) (*QuestionnaireResult, error) {
	l := logger.L(ctx)
	startTime := time.Now()

	l.Debugw("下架问卷",
		"action", "unpublish",
		"code", code,
	)

	q, err := s.loadUnpublishTarget(ctx, l, code)
	if err != nil {
		return nil, err
	}

	l.Debugw("执行下架流程",
		"action", "unpublish",
		"code", code,
		"current_status", q.GetStatus().String(),
	)
	if q.IsPublished() {
		if err := s.lifecycle.Unpublish(ctx, q); err != nil {
			l.Errorw("下架问卷失败",
				"action", "unpublish",
				"code", code,
				"result", "failed",
				"error", err.Error(),
			)
			return nil, err
		}
		if err := s.persistQuestionnaire(ctx, q, code, "unpublish", "状态"); err != nil {
			return nil, err
		}
	}
	if err := s.repo.ClearActivePublishedVersion(ctx, code); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "清理发布快照失败")
	}

	s.publishEvents(ctx, q)

	s.logSuccess(ctx, "unpublish", code, startTime,
		"status", q.GetStatus().String(),
	)

	return toQuestionnaireResult(q), nil
}

// Archive 归档问卷
func (s *lifecycleService) Archive(ctx context.Context, code string) (*QuestionnaireResult, error) {
	l := logger.L(ctx)
	startTime := time.Now()

	l.Debugw("归档问卷",
		"action", "archive",
		"code", code,
	)

	if err := s.validateCode(ctx, code, "archive"); err != nil {
		return nil, err
	}

	q, err := s.findQuestionnaireByCode(ctx, code, "archive")
	if err != nil {
		return nil, err
	}
	if q.IsArchived() {
		l.Warnw("问卷已归档，不能重复归档",
			"action", "archive",
			"code", code,
			"status", q.GetStatus().String(),
			"result", "invalid_status",
		)
		return nil, errors.WithCode(errorCode.ErrQuestionnaireInvalidStatus, "问卷已归档，不能重复归档")
	}

	l.Debugw("执行归档流程",
		"action", "archive",
		"code", code,
		"current_status", q.GetStatus().String(),
	)
	if err := s.lifecycle.Archive(ctx, q); err != nil {
		l.Errorw("归档问卷失败",
			"action", "archive",
			"code", code,
			"result", "failed",
			"error", err.Error(),
		)
		return nil, err
	}

	if err := s.persistQuestionnaire(ctx, q, code, "archive", "状态"); err != nil {
		return nil, err
	}
	if err := s.repo.ClearActivePublishedVersion(ctx, code); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "清理发布快照失败")
	}

	s.publishEvents(ctx, q)

	s.logSuccess(ctx, "archive", code, startTime,
		"status", q.GetStatus().String(),
	)

	return toQuestionnaireResult(q), nil
}

func (s *lifecycleService) loadUnpublishTarget(ctx context.Context, l *logger.RequestLogger, code string) (*domainQuestionnaire.Questionnaire, error) {
	if err := s.validateCode(ctx, code, "unpublish"); err != nil {
		return nil, err
	}

	q, err := s.findQuestionnaireByCode(ctx, code, "unpublish")
	if err != nil {
		return nil, err
	}

	if err := s.checkArchivedStatus(ctx, q, code, "unpublish", "下架"); err != nil {
		return nil, err
	}
	publishedQ, err := s.repo.FindPublishedByCode(ctx, code)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "获取已发布问卷失败")
	}
	if q.IsDraft() && publishedQ == nil {
		l.Warnw("问卷是草稿状态，不需要下架",
			"action", "unpublish",
			"code", code,
			"status", q.GetStatus().String(),
			"result", "invalid_status",
		)
		return nil, errors.WithCode(errorCode.ErrQuestionnaireInvalidStatus, "问卷是草稿状态，不需要下架")
	}

	return q, nil
}
