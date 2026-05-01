package answersheet

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
)

// createAndSaveAnswerSheet 创建并保存答卷。
func (s *submissionService) createAndSaveAnswerSheet(
	ctx context.Context,
	l *logger.RequestLogger,
	dto SubmitAnswerSheetDTO,
	qnr *questionnaire.Questionnaire,
	answers []answersheet.Answer,
) (*answersheet.AnswerSheet, error) {
	sheet, err := createAnswerSheet(l, dto, qnr, answers)
	if err != nil {
		return nil, err
	}
	return s.persistSubmittedAnswerSheet(ctx, l, dto, sheet)
}

func createAnswerSheet(
	l *logger.RequestLogger,
	dto SubmitAnswerSheetDTO,
	qnr *questionnaire.Questionnaire,
	answers []answersheet.Answer,
) (*answersheet.AnswerSheet, error) {
	questionnaireRef := answersheet.NewQuestionnaireRef(
		dto.QuestionnaireCode,
		dto.QuestionnaireVer,
		qnr.GetTitle(),
	)

	fillerUserID, err := fillerUserIDFromUint64("filler_id", dto.FillerID)
	if err != nil {
		return nil, err
	}
	fillerRef := actor.NewFillerRef(fillerUserID, actor.FillerTypeSelf)

	l.Debugw("开始创建答卷领域对象", "questionnaire_code", dto.QuestionnaireCode, "filler_id", dto.FillerID, "answer_count", len(answers))
	sheet, err := answersheet.NewAnswerSheet(questionnaireRef, fillerRef, answers, time.Now())
	if err != nil {
		l.Errorw("创建答卷领域对象失败", "questionnaire_code", dto.QuestionnaireCode, "error", err.Error(), "result", "failed")
		return nil, errors.WrapC(err, errorCode.ErrAnswerSheetInvalid, "创建答卷失败")
	}
	return sheet, nil
}

func (s *submissionService) persistSubmittedAnswerSheet(
	ctx context.Context,
	l *logger.RequestLogger,
	dto SubmitAnswerSheetDTO,
	sheet *answersheet.AnswerSheet,
) (*answersheet.AnswerSheet, error) {
	l.Infow("开始保存答卷", "action", "create", "resource", "answersheet", "questionnaire_code", dto.QuestionnaireCode)
	storedSheet, existing, err := s.durableStore.CreateDurably(ctx, sheet, DurableSubmitMeta{
		IdempotencyKey: dto.IdempotencyKey,
		WriterID:       dto.FillerID,
		TesteeID:       dto.TesteeID,
		OrgID:          dto.OrgID,
		TaskID:         dto.TaskID,
	})
	if err != nil {
		l.Errorw("保存答卷失败", "action", "create", "resource", "answersheet", "questionnaire_code", dto.QuestionnaireCode, "error", err.Error(), "result", "failed")
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存答卷失败")
	}
	if existing {
		l.Infow("答卷提交命中业务幂等键，返回已存在答卷",
			"action", "create",
			"resource", "answersheet",
			"idempotency_key", dto.IdempotencyKey,
			"answersheet_id", storedSheet.ID().Uint64(),
			"result", "idempotent_hit",
		)
	}
	return storedSheet, nil
}
