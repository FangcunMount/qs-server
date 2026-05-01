package questionnaire

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func (s *contentService) applyQuestionMutation(
	ctx context.Context,
	questionnaireCode string,
	action string,
	mutate func(*questionnaire.Questionnaire) error,
) (*questionnaire.Questionnaire, error) {
	q, err := s.loadEditableHead(ctx, questionnaireCode, action)
	if err != nil {
		return nil, err
	}
	if err := mutate(q); err != nil {
		return nil, err
	}
	if err := s.persistQuestionnaire(ctx, q, questionnaireCode, action); err != nil {
		return nil, err
	}
	return q, nil
}

func buildQuestionsFromDTOs(ctx context.Context, questionnaireCode string, questions []QuestionDTO) ([]questionnaire.Question, error) {
	domainQuestions := make([]questionnaire.Question, 0, len(questions))
	for i, qDTO := range questions {
		validationRules := toDomainValidationRules(qDTO.ValidationRules)
		calculationRule := toDomainCalculationRule(qDTO.CalculationRule)
		showController := toDomainShowController(qDTO.ShowController)
		question, err := buildQuestionFromDTO(qDTO.Code, qDTO.Stem, qDTO.Type, qDTO.Options, qDTO.Required, qDTO.Description, validationRules, calculationRule, showController)
		if err != nil {
			logger.L(ctx).Errorw("创建问题失败",
				"action", "batch_update_questions",
				"questionnaire_code", questionnaireCode,
				"question_index", i+1,
				"question_code", qDTO.Code,
				"result", "failed",
				"error", err.Error(),
			)
			return nil, errors.WrapC(err, errorCode.ErrQuestionnaireInvalidQuestion, "第 %d 个问题创建失败", i+1)
		}
		domainQuestions = append(domainQuestions, question)
	}
	return domainQuestions, nil
}

func toMetaCodes(codes []string) []meta.Code {
	metaCodes := make([]meta.Code, 0, len(codes))
	for _, code := range codes {
		metaCodes = append(metaCodes, meta.NewCode(code))
	}
	return metaCodes
}

// validateQuestionnaireCode 验证问卷编码
func (s *contentService) validateQuestionnaireCode(ctx context.Context, code string, action string) error {
	if code == "" {
		logger.L(ctx).Warnw("问卷编码为空",
			"action", action,
			"result", "invalid_params",
		)
		return errors.WithCode(errorCode.ErrQuestionnaireInvalidInput, "问卷编码不能为空")
	}
	return nil
}

// validateQuestionCode 验证问题编码
func (s *contentService) validateQuestionCode(ctx context.Context, code string, questionnaireCode string, action string) error {
	if code == "" {
		logger.L(ctx).Warnw("问题编码为空",
			"action", action,
			"questionnaire_code", questionnaireCode,
			"result", "invalid_params",
		)
		return errors.WithCode(errorCode.ErrQuestionnaireInvalidInput, "问题编码不能为空")
	}
	return nil
}

// findQuestionnaireByCode 根据编码查找问卷
func (s *contentService) findQuestionnaireByCode(ctx context.Context, code string, action string) (*questionnaire.Questionnaire, error) {
	q, err := s.repo.FindByCode(ctx, code)
	if err != nil {
		logger.L(ctx).Errorw("获取问卷失败",
			"action", action,
			"questionnaire_code", code,
			"result", "failed",
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrQuestionnaireNotFound, "获取问卷失败")
	}
	return q, nil
}

func (s *contentService) loadEditableHead(ctx context.Context, code string, action string) (*questionnaire.Questionnaire, error) {
	q, err := s.findQuestionnaireByCode(ctx, code, action)
	if err != nil {
		return nil, err
	}
	if err := s.checkArchivedStatus(ctx, q, code, action); err != nil {
		return nil, err
	}
	if err := ensureEditableHead(ctx, s.repo, q); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "派生工作草稿失败")
	}
	return q, nil
}

// checkArchivedStatus 检查问卷是否已归档
func (s *contentService) checkArchivedStatus(ctx context.Context, q *questionnaire.Questionnaire, code string, action string) error {
	if q.IsArchived() {
		logger.L(ctx).Warnw("问卷已归档，不能编辑",
			"action", action,
			"questionnaire_code", code,
			"status", q.GetStatus().String(),
			"result", "invalid_status",
		)
		return errors.WithCode(errorCode.ErrQuestionnaireArchived, "问卷已归档，不能编辑")
	}
	return nil
}

// persistQuestionnaire 持久化问卷
func (s *contentService) persistQuestionnaire(ctx context.Context, q *questionnaire.Questionnaire, code string, action string) error {
	if err := s.repo.Update(ctx, q); err != nil {
		logger.L(ctx).Errorw("保存问卷失败",
			"action", action,
			"questionnaire_code", code,
			"result", "failed",
			"error", err.Error(),
		)
		return errors.WrapC(err, errorCode.ErrDatabase, "保存问卷失败")
	}
	return nil
}

// logSuccess 记录成功日志
func (s *contentService) logSuccess(ctx context.Context, action string, questionnaireCode string, startTime time.Time, extraFields ...interface{}) {
	duration := time.Since(startTime)
	fields := []interface{}{
		"action", action,
		"questionnaire_code", questionnaireCode,
		"duration_ms", duration.Milliseconds(),
	}
	fields = append(fields, extraFields...)
	logger.L(ctx).Debugw("操作成功", fields...)
}
