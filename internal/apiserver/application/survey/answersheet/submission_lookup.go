package answersheet

import (
	"context"
	"fmt"

	"github.com/FangcunMount/component-base/pkg/errors"
	domainanswersheet "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	submitport "github.com/FangcunMount/qs-server/internal/apiserver/port/answersheetsubmit"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
)

func (s *submissionService) LookupAcceptedSubmission(ctx context.Context, dto LookupSubmissionDTO) (*AnswerSheetResult, bool, error) {
	reader, ok := s.durableStore.(SubmissionIdempotencyReader)
	if !ok || reader == nil {
		observeDurableOperation("explicit_readback", "error")
		return nil, false, errors.WithCode(errorCode.ErrDatabase, "答卷持久结果回读不可用")
	}
	if err := validateLookupSubmissionDTO(dto); err != nil {
		observeDurableOperation("explicit_readback", "invalid")
		return nil, false, err
	}

	completed, err := reader.FindCompleted(ctx, DurableSubmitMeta{
		IdempotencyKey: dto.IdempotencyKey,
		WriterID:       dto.FillerID,
	})
	if err != nil {
		observeDurableOperation("explicit_readback", "error")
		return nil, false, errors.WrapC(err, errorCode.ErrDatabase, "回读已受理答卷失败")
	}
	if completed == nil {
		observeDurableOperation("explicit_readback", "miss")
		return nil, false, nil
	}
	if err := validateCompletedSubmission(completed); err != nil {
		observeDurableOperation("explicit_readback", "error")
		return nil, false, errors.WithCode(errorCode.ErrDatabase, "已受理答卷持久结果不完整")
	}

	candidateFingerprint, err := lookupSubmissionFingerprint(completed.Sheet, dto)
	if err != nil {
		observeDurableOperation("explicit_readback", "invalid")
		return nil, false, err
	}
	if candidateFingerprint != completed.Fingerprint {
		observeDurableOperation("explicit_readback", "conflict")
		return nil, false, errors.WithCode(errorCode.ErrConflict, "%v", submitport.ErrIdempotencyConflict)
	}

	observeDurableOperation("explicit_readback", "hit")
	observeDurableSubmit("idempotency_hit")
	return toAnswerSheetResult(completed.Sheet), true, nil
}

func validateLookupSubmissionDTO(dto LookupSubmissionDTO) error {
	switch {
	case dto.FillerID == 0:
		return errors.WithCode(errorCode.ErrAnswerSheetInvalid, "填写人ID不能为空")
	case dto.TesteeID == 0:
		return errors.WithCode(errorCode.ErrAnswerSheetInvalid, "受试者ID不能为空")
	case dto.IdempotencyKey == "":
		return errors.WithCode(errorCode.ErrAnswerSheetInvalid, "幂等键不能为空")
	case dto.QuestionnaireCode == "":
		return errors.WithCode(errorCode.ErrAnswerSheetInvalid, "问卷编码不能为空")
	case dto.QuestionnaireVer == "":
		return errors.WithCode(errorCode.ErrAnswerSheetInvalid, "问卷版本不能为空")
	case len(dto.Answers) == 0:
		return errors.WithCode(errorCode.ErrAnswerSheetInvalid, "答案列表不能为空")
	}
	return nil
}

func lookupSubmissionFingerprint(existing *domainanswersheet.AnswerSheet, dto LookupSubmissionDTO) (string, error) {
	writerID, err := fillerUserIDFromUint64("filler_id", dto.FillerID)
	if err != nil {
		return "", err
	}
	ref, err := originRefFromDTO(SubmitAnswerSheetDTO{
		TaskID:    dto.TaskID,
		OriginRef: dto.OriginRef,
	})
	if err != nil {
		return "", err
	}
	answers := make([]submitport.SubmissionAnswer, 0, len(dto.Answers))
	for index, answer := range dto.Answers {
		if answer.QuestionCode == "" || answer.QuestionType == "" {
			return "", errors.WithCode(errorCode.ErrAnswerSheetInvalid, "第 %d 个答案缺少问题编码或类型", index+1)
		}
		value, valueErr := domainanswersheet.CreateAnswerValueFromRaw(questionnaire.QuestionType(answer.QuestionType), answer.Value)
		if valueErr != nil {
			return "", errors.WithCode(errorCode.ErrAnswerSheetInvalid, "%s", fmt.Sprintf("问题 [%s] 的答案格式不正确: %v", answer.QuestionCode, valueErr))
		}
		answers = append(answers, submitport.SubmissionAnswer{
			QuestionCode: answer.QuestionCode,
			QuestionType: answer.QuestionType,
			Value:        value.Raw(),
		})
	}
	fingerprint, err := submitport.FingerprintIntent(submitport.SubmissionIntent{
		WriterID:             writerID,
		TesteeID:             dto.TesteeID,
		OrgID:                existing.SubmissionContext().OrgID().Uint64(),
		TaskID:               dto.TaskID,
		OriginType:           string(ref.Type),
		OriginID:             ref.ID,
		QuestionnaireCode:    dto.QuestionnaireCode,
		QuestionnaireVersion: dto.QuestionnaireVer,
		Answers:              answers,
	})
	if err != nil {
		return "", errors.WrapC(err, errorCode.ErrAnswerSheetInvalid, "计算答卷提交指纹失败")
	}
	return fingerprint, nil
}
