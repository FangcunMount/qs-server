package answersheet

import (
	"context"
	stderrors "errors"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	submitport "github.com/FangcunMount/qs-server/internal/apiserver/port/answersheetsubmit"
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
	admission, err := s.resolveAdmission(ctx, dto.QuestionnaireCode, dto.QuestionnaireVer)
	if err != nil {
		return nil, err
	}
	if !admission.IsZero() {
		l.Infow("答卷准入意图已冻结",
			"action", "submit_answersheet",
			"stage", "admission",
			"purpose", string(admission.Purpose()),
			"model_code", admission.ModelCode(),
			"model_version", admission.ModelVersion(),
		)
	}
	sheet, err := createAnswerSheet(l, dto, qnr, answers, admission)
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
	admission answersheet.Admission,
) (*answersheet.AnswerSheet, error) {
	questionnaireRef, err := answersheet.NewQuestionnaireRef(
		dto.QuestionnaireCode,
		dto.QuestionnaireVer,
		qnr.GetTitle(),
	)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrAnswerSheetInvalid, "创建问卷引用失败")
	}

	fillerUserID, err := fillerUserIDFromUint64("filler_id", dto.FillerID)
	if err != nil {
		return nil, err
	}
	fillerRef := actor.NewFillerRef(fillerUserID, actor.FillerTypeSelf)
	testeeID, err := metaIDFromUint64("testee_id", dto.TesteeID)
	if err != nil {
		return nil, err
	}
	orgID, err := metaIDFromUint64("org_id", dto.OrgID)
	if err != nil {
		return nil, err
	}
	var submissionContext answersheet.SubmissionContext
	if admission.IsZero() {
		submissionContext, err = answersheet.NewSubmissionContext(
			fillerRef,
			actor.NewTesteeRef(testeeID),
			orgID,
			dto.TaskID,
		)
	} else {
		submissionContext, err = answersheet.NewSubmissionContext(
			fillerRef,
			actor.NewTesteeRef(testeeID),
			orgID,
			dto.TaskID,
			admission,
		)
	}
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrAnswerSheetInvalid, "创建答卷提交上下文失败")
	}

	l.Debugw("开始创建答卷领域对象", "questionnaire_code", dto.QuestionnaireCode, "filler_id", dto.FillerID, "answer_count", len(answers))
	sheet, err := answersheet.Submit(answersheet.NewID(), questionnaireRef, submissionContext, answers, time.Now())
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
	l.Infow("开始保存答卷", "action", "submit_answersheet", "stage", "durable_transaction", "result", "started",
		"resource", "answersheet", "request_id", dto.RequestID, "questionnaire_code", dto.QuestionnaireCode)
	fingerprint, err := submitport.Fingerprint(sheet)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrAnswerSheetInvalid, "计算答卷提交指纹失败")
	}
	storedSheet, existing, err := s.durableStore.CreateDurably(ctx, sheet, DurableSubmitMeta{
		IdempotencyKey: dto.IdempotencyKey,
		WriterID:       dto.FillerID,
		Fingerprint:    fingerprint,
		RequestID:      dto.RequestID,
	})
	if err != nil {
		if stderrors.Is(err, submitport.ErrIdempotencyConflict) {
			observeDurableSubmit("idempotency_conflict")
			l.Warnw("答卷幂等键与已保存业务内容冲突",
				"action", "submit_answersheet", "stage", "durable_transaction", "result", "idempotency_conflict",
				"error_category", "idempotency_conflict", "resource", "answersheet", "request_id", dto.RequestID,
				"writer_id", dto.FillerID, "idempotency_key", dto.IdempotencyKey,
				"questionnaire_code", dto.QuestionnaireCode, "testee_id", dto.TesteeID,
			)
			return nil, errors.WithCode(errorCode.ErrConflict, "%v", err)
		}
		observeDurableSubmit("failed")
		l.Errorw("保存答卷失败", "action", "submit_answersheet", "stage", "durable_transaction", "result", "failed",
			"error_category", "dependency_unavailable", "resource", "answersheet", "request_id", dto.RequestID,
			"questionnaire_code", dto.QuestionnaireCode, "error", err.Error())
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存答卷失败")
	}
	if existing {
		observeDurableSubmit("idempotency_hit")
		l.Infow("答卷提交命中业务幂等键，返回已存在答卷",
			"action", "submit_answersheet",
			"stage", "durable_transaction",
			"resource", "answersheet",
			"request_id", dto.RequestID,
			"idempotency_key", dto.IdempotencyKey,
			"answersheet_id", storedSheet.ID().Uint64(),
			"result", "idempotent_hit",
		)
	}
	if !existing {
		observeDurableSubmit("created")
		l.Infow("答卷可靠事务已提交",
			"action", "submit_answersheet", "stage", "durable_transaction", "result", "created",
			"resource", "answersheet", "request_id", dto.RequestID,
			"answersheet_id", storedSheet.ID().Uint64(), "questionnaire_code", dto.QuestionnaireCode,
		)
	}
	return storedSheet, nil
}
