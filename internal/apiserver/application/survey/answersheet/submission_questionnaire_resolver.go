package answersheet

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
)

// fetchAndValidateQuestionnaire 获取并验证问卷，返回问卷对象和问题映射表。
func (s *submissionService) fetchAndValidateQuestionnaire(
	ctx context.Context,
	l *logger.RequestLogger,
	dto *SubmitAnswerSheetDTO,
) (*questionnaire.Questionnaire, map[string]questionnaire.Question, error) {
	l.Debugw("开始获取问卷信息", "questionnaire_code", dto.QuestionnaireCode, "action", "read", "resource", "questionnaire")

	qnr, err := s.resolveSubmittableQuestionnaire(ctx, l, dto)
	if err != nil {
		return nil, nil, err
	}

	l.Debugw("问卷信息获取成功", "questionnaire_code", dto.QuestionnaireCode, "questionnaire_title", qnr.GetTitle(), "question_count", len(qnr.GetQuestions()), "questionnaire_version", dto.QuestionnaireVer, "result", "success")

	questionMap := questionnaireMapByCode(qnr.GetQuestions())
	l.Debugw("问卷验证通过", "questionnaire_code", dto.QuestionnaireCode, "version", dto.QuestionnaireVer, "question_count", len(questionMap))
	return qnr, questionMap, nil
}

func (s *submissionService) resolveSubmittableQuestionnaire(
	ctx context.Context,
	l *logger.RequestLogger,
	dto *SubmitAnswerSheetDTO,
) (*questionnaire.Questionnaire, error) {
	if dto.QuestionnaireVer == "" {
		qnr, err := s.questionnaireRepo.FindPublishedByCode(ctx, dto.QuestionnaireCode)
		if err != nil {
			l.Errorw("获取已发布问卷失败", "questionnaire_code", dto.QuestionnaireCode, "action", "read", "resource", "questionnaire", "result", "failed", "error", err.Error())
			return nil, errors.WrapC(err, errorCode.ErrQuestionnaireNotFound, "问卷不存在")
		}
		if qnr == nil {
			return nil, errors.WithCode(errorCode.ErrAnswerSheetInvalid, "当前没有可提交的已发布问卷版本")
		}
		dto.QuestionnaireVer = qnr.GetVersion().Value()
		l.Debugw("使用当前已发布问卷版本", "questionnaire_code", dto.QuestionnaireCode, "version", dto.QuestionnaireVer)
		return qnr, nil
	}

	qnr, err := s.questionnaireRepo.FindByCodeVersion(ctx, dto.QuestionnaireCode, dto.QuestionnaireVer)
	if err != nil {
		l.Errorw("获取指定问卷版本失败", "questionnaire_code", dto.QuestionnaireCode, "questionnaire_version", dto.QuestionnaireVer, "action", "read", "resource", "questionnaire", "result", "failed", "error", err.Error())
		return nil, errors.WrapC(err, errorCode.ErrQuestionnaireNotFound, "问卷不存在")
	}
	if qnr == nil || !qnr.IsPublished() {
		return nil, errors.WithCode(errorCode.ErrAnswerSheetInvalid, "只能提交已发布的问卷版本")
	}
	return qnr, nil
}

func questionnaireMapByCode(questions []questionnaire.Question) map[string]questionnaire.Question {
	questionMap := make(map[string]questionnaire.Question, len(questions))
	for _, q := range questions {
		questionMap[q.GetCode().Value()] = q
	}
	return questionMap
}
