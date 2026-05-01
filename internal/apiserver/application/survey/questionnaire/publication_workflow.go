package questionnaire

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	domainQuestionnaire "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
)

func (s *lifecycleService) publishQuestionnaireVersion(
	ctx context.Context,
	l *logger.RequestLogger,
	q *domainQuestionnaire.Questionnaire,
	code string,
) error {
	if err := s.ensurePublishable(ctx, l, q, code); err != nil {
		return err
	}
	if err := s.applyPublishLifecycle(ctx, l, q, code); err != nil {
		return err
	}
	if err := s.persistPublishedQuestionnaire(ctx, q, code); err != nil {
		return err
	}
	return s.syncScaleQuestionnaireVersion(ctx, code, q.GetVersion().String())
}

func (s *lifecycleService) ensurePublishable(
	ctx context.Context,
	l *logger.RequestLogger,
	q *domainQuestionnaire.Questionnaire,
	code string,
) error {
	questionsCount := len(q.GetQuestions())
	l.Debugw("检查问题列表",
		"action", "publish",
		"code", code,
		"questions_count", questionsCount,
	)
	if questionsCount == 0 {
		l.Warnw("问卷没有问题，不能发布",
			"action", "publish",
			"code", code,
			"result", "invalid_question",
		)
		return errors.WithCode(errorCode.ErrQuestionnaireInvalidQuestion, "问卷没有问题，不能发布")
	}
	return nil
}

func (s *lifecycleService) applyPublishLifecycle(
	ctx context.Context,
	l *logger.RequestLogger,
	q *domainQuestionnaire.Questionnaire,
	code string,
) error {
	l.Debugw("执行发布流程",
		"action", "publish",
		"code", code,
		"current_version", q.GetVersion().String(),
	)
	if err := s.lifecycle.Publish(ctx, q); err != nil {
		l.Errorw("发布问卷失败",
			"action", "publish",
			"code", code,
			"result", "failed",
			"error", err.Error(),
		)
		return err
	}
	return nil
}

func (s *lifecycleService) persistPublishedQuestionnaire(ctx context.Context, q *domainQuestionnaire.Questionnaire, code string) error {
	if err := s.persistQuestionnaire(ctx, q, code, "publish", "状态"); err != nil {
		return err
	}
	if err := s.repo.CreatePublishedSnapshot(ctx, q, true); err != nil {
		return errors.WrapC(err, errorCode.ErrDatabase, "保存发布快照失败")
	}
	if err := s.repo.SetActivePublishedVersion(ctx, code, q.GetVersion().String()); err != nil {
		return errors.WrapC(err, errorCode.ErrDatabase, "切换发布快照失败")
	}
	return nil
}
