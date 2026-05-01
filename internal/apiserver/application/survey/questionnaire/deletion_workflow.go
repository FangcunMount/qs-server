package questionnaire

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
)

func (s *lifecycleService) deleteQuestionnaire(ctx context.Context, l *logger.RequestLogger, code string) error {
	if err := s.validateCode(ctx, code, "delete"); err != nil {
		return err
	}

	q, err := s.findQuestionnaireByCode(ctx, code, "delete")
	if err != nil {
		return err
	}
	if !q.IsDraft() {
		l.Warnw("只能删除草稿状态的问卷",
			"action", "delete",
			"code", code,
			"status", q.GetStatus().String(),
			"result", "invalid_status",
		)
		return errors.WithCode(errorCode.ErrQuestionnaireInvalidStatus, "只能删除草稿状态的问卷")
	}

	hasSnapshots, err := s.repo.HasPublishedSnapshots(ctx, code)
	if err != nil {
		return errors.WrapC(err, errorCode.ErrDatabase, "查询发布快照失败")
	}
	if !hasSnapshots {
		return s.deleteQuestionnaireFamily(ctx, l, code)
	}
	return s.deleteDraftAndRestoreLatestPublished(ctx, code)
}

func (s *lifecycleService) deleteQuestionnaireFamily(ctx context.Context, l *logger.RequestLogger, code string) error {
	l.Debugw("删除整个问卷族",
		"action", "delete",
		"code", code,
	)
	if err := s.repo.HardDeleteFamily(ctx, code); err != nil {
		l.Errorw("删除问卷失败",
			"action", "delete",
			"code", code,
			"result", "failed",
			"error", err.Error(),
		)
		return errors.WrapC(err, errorCode.ErrDatabase, "删除问卷失败")
	}
	return nil
}

func (s *lifecycleService) deleteDraftAndRestoreLatestPublished(ctx context.Context, code string) error {
	latestPublished, err := s.repo.FindLatestPublishedByCode(ctx, code)
	if err != nil {
		return errors.WrapC(err, errorCode.ErrDatabase, "加载历史发布快照失败")
	}
	if err := s.repo.HardDelete(ctx, code); err != nil {
		return errors.WrapC(err, errorCode.ErrDatabase, "删除工作版本失败")
	}
	if latestPublished == nil {
		return nil
	}

	restored, err := cloneQuestionnaireAsHead(latestPublished)
	if err != nil {
		return errors.WrapC(err, errorCode.ErrQuestionnaireInvalidInput, "恢复工作版本失败")
	}
	if err := s.repo.Update(ctx, restored); err != nil {
		return errors.WrapC(err, errorCode.ErrDatabase, "恢复工作版本失败")
	}
	return nil
}
