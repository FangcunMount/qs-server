package questionnaire

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	domainQuestionnaire "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
)

func (s *lifecycleService) saveDraftQuestionnaire(
	ctx context.Context,
	l *logger.RequestLogger,
	code string,
) (*domainQuestionnaire.Questionnaire, error) {
	if err := s.validateCode(ctx, code, "save_draft"); err != nil {
		return nil, err
	}

	q, err := s.findQuestionnaireByCode(ctx, code, "save_draft")
	if err != nil {
		return nil, err
	}

	if !q.IsDraft() {
		l.Warnw("只能保存草稿状态的问卷",
			"action", "save_draft",
			"code", code,
			"status", q.GetStatus().String(),
			"result", "invalid_status",
		)
		return nil, errors.WithCode(errorCode.ErrQuestionnaireInvalidStatus, "只能保存草稿状态的问卷")
	}

	oldVersion := q.GetVersion().String()
	l.Debugw("递增小版本号",
		"action", "save_draft",
		"code", code,
		"old_version", oldVersion,
	)
	versioning := domainQuestionnaire.Versioning{}
	if err := versioning.IncrementMinorVersion(q); err != nil {
		l.Errorw("更新版本号失败",
			"action", "save_draft",
			"code", code,
			"result", "failed",
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrQuestionnaireInvalidInput, "更新版本号失败")
	}

	if err := s.persistQuestionnaire(ctx, q, code, "save_draft", "草稿"); err != nil {
		return nil, err
	}
	return q, nil
}
