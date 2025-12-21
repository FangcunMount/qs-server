package answersheet

import (
	"context"
	"fmt"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// AnswerSheetScoringService 答卷计分应用服务
type AnswerSheetScoringService interface {
	// CalculateAndSave 计算并保存答卷分数
	CalculateAndSave(ctx context.Context, answerSheetID uint64) error
}

type answerSheetScoringService struct {
	answerSheetRepo   answersheet.Repository
	questionnaireRepo questionnaire.Repository
	scoringService    answersheet.ScoringService
}

// NewAnswerSheetScoringService 创建答卷计分应用服务
func NewAnswerSheetScoringService(
	answerSheetRepo answersheet.Repository,
	questionnaireRepo questionnaire.Repository,
	scoringService answersheet.ScoringService,
) AnswerSheetScoringService {
	return &answerSheetScoringService{
		answerSheetRepo:   answerSheetRepo,
		questionnaireRepo: questionnaireRepo,
		scoringService:    scoringService,
	}
}

// CalculateAndSave 计算并保存答卷分数
func (s *answerSheetScoringService) CalculateAndSave(ctx context.Context, answerSheetID uint64) error {
	l := logger.L(ctx)

	l.Infow("开始计算答卷分数",
		"action", "calculate_score",
		"resource", "answersheet",
		"answersheet_id", answerSheetID,
	)

	// 参数校验
	if answerSheetID == 0 {
		l.Warnw("答卷ID为空", "action", "calculate_score", "result", "invalid_params")
		return errors.WithCode(errorCode.ErrInvalidArgument, "答卷ID不能为空")
	}

	// 1. 加载答卷
	sheet, err := s.answerSheetRepo.FindByID(ctx, meta.ID(answerSheetID))
	if err != nil {
		l.Errorw("加载答卷失败", "answersheet_id", answerSheetID, "error", err.Error())
		return errors.WrapC(err, errorCode.ErrAnswerSheetNotFound, "答卷不存在")
	}

	l.Debugw("答卷加载成功", "answersheet_id", answerSheetID, "questionnaire_code", sheet.QuestionnaireRef().Code(), "answer_count", len(sheet.Answers()))

	// 2. 加载问卷
	qnr, err := s.questionnaireRepo.FindByCode(ctx, sheet.QuestionnaireRef().Code())
	if err != nil {
		l.Errorw("加载问卷失败", "questionnaire_code", sheet.QuestionnaireRef().Code(), "error", err.Error())
		return errors.WrapC(err, errorCode.ErrQuestionnaireNotFound, "问卷不存在")
	}

	l.Debugw("问卷加载成功", "questionnaire_code", qnr.GetCode().Value(), "question_count", len(qnr.GetQuestions()))

	// 3. 计算分数
	scoredSheet, err := s.scoringService.CalculateAnswerSheetScore(ctx, sheet, qnr)
	if err != nil {
		l.Errorw("计算分数失败", "answersheet_id", answerSheetID, "error", err.Error())
		return errors.WrapC(err, errorCode.ErrAnswerSheetScoreCalculationFailed, "计算分数失败")
	}

	l.Debugw("分数计算完成", "answersheet_id", answerSheetID, "total_score", scoredSheet.TotalScore, "scored_answer_count", len(scoredSheet.ScoredAnswers))

	// 调试：如果总分为0，记录每个答案的分数详情
	if scoredSheet.TotalScore == 0 && len(scoredSheet.ScoredAnswers) > 0 {
		for _, scoredAns := range scoredSheet.ScoredAnswers {
			if scoredAns.Score == 0 {
				// 查找对应的答案和问题
				var answerValue interface{}
				for _, ans := range sheet.Answers() {
					if ans.QuestionCode() == scoredAns.QuestionCode {
						answerValue = ans.Value().Raw()
						break
					}
				}
				// 查找问题选项
				var optionScores map[string]float64
				for _, q := range qnr.GetQuestions() {
					if q.GetCode().Value() == scoredAns.QuestionCode {
						options := q.GetOptions()
						optionScores = make(map[string]float64, len(options))
						for _, opt := range options {
							optionScores[opt.GetCode().Value()] = opt.GetScore()
						}
						break
					}
				}
				// 检查答案值是否在选项分数映射中
				answerValueStr, isString := answerValue.(string)
				matched := false
				if isString && answerValueStr != "" {
					_, matched = optionScores[answerValueStr]
				}

				l.Debugw("答案分数为0的详情",
					"question_code", scoredAns.QuestionCode,
					"answer_value", answerValue,
					"answer_value_type", fmt.Sprintf("%T", answerValue),
					"option_scores", optionScores,
					"matched", matched,
					"score", scoredAns.Score)
			}
		}
	}

	// 4. 更新答卷分数
	if err := sheet.UpdateScores(scoredSheet); err != nil {
		l.Errorw("更新答卷分数失败", "answersheet_id", answerSheetID, "error", err.Error())
		return errors.WrapC(err, errorCode.ErrAnswerSheetInvalid, "更新分数失败")
	}

	// 5. 持久化
	if err := s.answerSheetRepo.Update(ctx, sheet); err != nil {
		l.Errorw("保存答卷失败", "answersheet_id", answerSheetID, "error", err.Error())
		return errors.WrapC(err, errorCode.ErrDatabase, "保存答卷失败")
	}

	l.Infow("答卷计分完成", "action", "calculate_score", "resource", "answersheet", "result", "success",
		"answersheet_id", answerSheetID, "total_score", scoredSheet.TotalScore, "scored_answer_count", len(scoredSheet.ScoredAnswers),
	)

	return nil
}
