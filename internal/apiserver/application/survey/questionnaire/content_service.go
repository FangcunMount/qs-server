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

// contentService 问卷内容编辑服务实现
// 行为者：问卷内容编辑者
type contentService struct {
	repo questionnaire.Repository
}

// NewContentService 创建问卷内容编辑服务
func NewContentService(repo questionnaire.Repository) QuestionnaireContentService {
	return &contentService{
		repo: repo,
	}
}

// AddQuestion 添加问题
func (s *contentService) AddQuestion(ctx context.Context, dto AddQuestionDTO) (*QuestionnaireResult, error) {
	l := logger.L(ctx)
	startTime := time.Now()

	l.Debugw("添加问题",
		"action", "add_question",
		"questionnaire_code", dto.QuestionnaireCode,
		"question_code", dto.Code,
		"question_type", dto.Type,
		"options_count", len(dto.Options),
	)

	// 1. 验证输入参数
	if err := s.validateQuestionnaireCode(ctx, dto.QuestionnaireCode, "add_question"); err != nil {
		return nil, err
	}
	if err := s.validateQuestionCode(ctx, dto.Code, dto.QuestionnaireCode, "add_question"); err != nil {
		return nil, err
	}
	if dto.Stem == "" {
		l.Warnw("问题题干为空",
			"action", "add_question",
			"questionnaire_code", dto.QuestionnaireCode,
			"question_code", dto.Code,
			"result", "invalid_params",
		)
		return nil, errors.WithCode(errorCode.ErrQuestionnaireInvalidInput, "问题题干不能为空")
	}
	if dto.Type == "" {
		l.Warnw("问题类型为空",
			"action", "add_question",
			"questionnaire_code", dto.QuestionnaireCode,
			"question_code", dto.Code,
			"result", "invalid_params",
		)
		return nil, errors.WithCode(errorCode.ErrQuestionnaireInvalidInput, "问题类型不能为空")
	}

	q, err := s.applyQuestionMutation(ctx, dto.QuestionnaireCode, "add_question", func(q *questionnaire.Questionnaire) error {
		question, err := buildQuestionFromDTO(dto.Code, dto.Stem, dto.Type, dto.Options, dto.Required, dto.Description, nil, nil, nil)
		if err != nil {
			l.Errorw("创建问题失败",
				"action", "add_question",
				"questionnaire_code", dto.QuestionnaireCode,
				"question_code", dto.Code,
				"result", "failed",
				"error", err.Error(),
			)
			return errors.WrapC(err, errorCode.ErrQuestionnaireInvalidQuestion, "创建问题失败")
		}
		if err := q.AddQuestion(question); err != nil {
			l.Errorw("添加问题失败",
				"action", "add_question",
				"questionnaire_code", dto.QuestionnaireCode,
				"question_code", dto.Code,
				"result", "failed",
				"error", err.Error(),
			)
			return errors.WrapC(err, errorCode.ErrQuestionnaireInvalidQuestion, "添加问题失败")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	s.logSuccess(ctx, "add_question", dto.QuestionnaireCode, startTime,
		"question_code", dto.Code,
		"questions_count", len(q.GetQuestions()),
	)

	return toQuestionnaireResult(q), nil
}

// UpdateQuestion 更新问题
func (s *contentService) UpdateQuestion(ctx context.Context, dto UpdateQuestionDTO) (*QuestionnaireResult, error) {
	l := logger.L(ctx)
	startTime := time.Now()

	l.Debugw("更新问题",
		"action", "update_question",
		"questionnaire_code", dto.QuestionnaireCode,
		"question_code", dto.Code,
		"question_type", dto.Type,
		"options_count", len(dto.Options),
	)

	// 1. 验证输入参数
	if err := s.validateQuestionnaireCode(ctx, dto.QuestionnaireCode, "update_question"); err != nil {
		return nil, err
	}
	if err := s.validateQuestionCode(ctx, dto.Code, dto.QuestionnaireCode, "update_question"); err != nil {
		return nil, err
	}
	if dto.Stem == "" {
		l.Warnw("问题题干为空",
			"action", "update_question",
			"questionnaire_code", dto.QuestionnaireCode,
			"question_code", dto.Code,
			"result", "invalid_params",
		)
		return nil, errors.WithCode(errorCode.ErrQuestionnaireInvalidInput, "问题题干不能为空")
	}

	q, err := s.applyQuestionMutation(ctx, dto.QuestionnaireCode, "update_question", func(q *questionnaire.Questionnaire) error {
		newQuestion, err := buildQuestionFromDTO(dto.Code, dto.Stem, dto.Type, dto.Options, dto.Required, dto.Description, nil, nil, nil)
		if err != nil {
			l.Errorw("创建问题失败",
				"action", "update_question",
				"questionnaire_code", dto.QuestionnaireCode,
				"question_code", dto.Code,
				"result", "failed",
				"error", err.Error(),
			)
			return errors.WrapC(err, errorCode.ErrQuestionnaireInvalidQuestion, "创建问题失败")
		}
		if err := q.UpdateQuestion(newQuestion); err != nil {
			l.Errorw("更新问题失败",
				"action", "update_question",
				"questionnaire_code", dto.QuestionnaireCode,
				"question_code", dto.Code,
				"result", "failed",
				"error", err.Error(),
			)
			return errors.WrapC(err, errorCode.ErrQuestionnaireInvalidQuestion, "更新问题失败")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	s.logSuccess(ctx, "update_question", dto.QuestionnaireCode, startTime,
		"question_code", dto.Code,
	)

	return toQuestionnaireResult(q), nil
}

// RemoveQuestion 删除问题
func (s *contentService) RemoveQuestion(ctx context.Context, questionnaireCode, questionCode string) (*QuestionnaireResult, error) {
	l := logger.L(ctx)
	startTime := time.Now()

	l.Debugw("删除问题",
		"action", "remove_question",
		"questionnaire_code", questionnaireCode,
		"question_code", questionCode,
	)

	// 1. 验证输入参数
	if err := s.validateQuestionnaireCode(ctx, questionnaireCode, "remove_question"); err != nil {
		return nil, err
	}
	if err := s.validateQuestionCode(ctx, questionCode, questionnaireCode, "remove_question"); err != nil {
		return nil, err
	}

	q, err := s.applyQuestionMutation(ctx, questionnaireCode, "remove_question", func(q *questionnaire.Questionnaire) error {
		if err := q.RemoveQuestion(meta.NewCode(questionCode)); err != nil {
			l.Errorw("删除问题失败",
				"action", "remove_question",
				"questionnaire_code", questionnaireCode,
				"question_code", questionCode,
				"result", "failed",
				"error", err.Error(),
			)
			return errors.WrapC(err, errorCode.ErrQuestionnaireInvalidQuestion, "删除问题失败")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	s.logSuccess(ctx, "remove_question", questionnaireCode, startTime,
		"question_code", questionCode,
		"questions_count", len(q.GetQuestions()),
	)

	return toQuestionnaireResult(q), nil
}

// ReorderQuestions 重排问题顺序
func (s *contentService) ReorderQuestions(ctx context.Context, questionnaireCode string, orderedCodes []string) (*QuestionnaireResult, error) {
	l := logger.L(ctx)
	startTime := time.Now()

	l.Debugw("重排问题顺序",
		"action", "reorder_questions",
		"questionnaire_code", questionnaireCode,
		"ordered_codes_count", len(orderedCodes),
	)

	// 1. 验证输入参数
	if err := s.validateQuestionnaireCode(ctx, questionnaireCode, "reorder_questions"); err != nil {
		return nil, err
	}
	if len(orderedCodes) == 0 {
		l.Warnw("问题编码列表为空",
			"action", "reorder_questions",
			"questionnaire_code", questionnaireCode,
			"result", "invalid_params",
		)
		return nil, errors.WithCode(errorCode.ErrQuestionnaireInvalidInput, "问题编码列表不能为空")
	}

	q, err := s.applyQuestionMutation(ctx, questionnaireCode, "reorder_questions", func(q *questionnaire.Questionnaire) error {
		if err := q.ReorderQuestions(toMetaCodes(orderedCodes)); err != nil {
			l.Errorw("重排问题顺序失败",
				"action", "reorder_questions",
				"questionnaire_code", questionnaireCode,
				"result", "failed",
				"error", err.Error(),
			)
			return errors.WrapC(err, errorCode.ErrQuestionnaireInvalidQuestion, "重排问题顺序失败")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	s.logSuccess(ctx, "reorder_questions", questionnaireCode, startTime,
		"questions_count", len(q.GetQuestions()),
	)

	return toQuestionnaireResult(q), nil
}

// BatchUpdateQuestions 批量更新问题
func (s *contentService) BatchUpdateQuestions(ctx context.Context, questionnaireCode string, questions []QuestionDTO) (*QuestionnaireResult, error) {
	l := logger.L(ctx)
	startTime := time.Now()

	l.Debugw("批量更新问题",
		"action", "batch_update_questions",
		"questionnaire_code", questionnaireCode,
		"questions_count", len(questions),
	)

	// 1. 验证输入参数
	if err := s.validateQuestionnaireCode(ctx, questionnaireCode, "batch_update_questions"); err != nil {
		return nil, err
	}
	if len(questions) == 0 {
		l.Warnw("问题列表为空",
			"action", "batch_update_questions",
			"questionnaire_code", questionnaireCode,
			"result", "invalid_params",
		)
		return nil, errors.WithCode(errorCode.ErrQuestionnaireInvalidInput, "问题列表不能为空")
	}

	var domainQuestions []questionnaire.Question
	q, err := s.applyQuestionMutation(ctx, questionnaireCode, "batch_update_questions", func(q *questionnaire.Questionnaire) error {
		var err error
		domainQuestions, err = buildQuestionsFromDTOs(ctx, questionnaireCode, questions)
		if err != nil {
			return err
		}
		if err := q.ReplaceQuestions(domainQuestions); err != nil {
			l.Errorw("批量更新问题失败",
				"action", "batch_update_questions",
				"questionnaire_code", questionnaireCode,
				"result", "failed",
				"error", err.Error(),
			)
			return errors.WrapC(err, errorCode.ErrQuestionnaireInvalidQuestion, "批量更新问题失败")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	s.logSuccess(ctx, "batch_update_questions", questionnaireCode, startTime,
		"questions_count", len(domainQuestions),
	)

	return toQuestionnaireResult(q), nil
}
