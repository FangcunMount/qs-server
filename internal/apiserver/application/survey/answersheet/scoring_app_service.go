package answersheet

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/ruleengine"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
)

// AnswerSheetScoringService 答卷计分应用服务
type AnswerSheetScoringService interface {
	// CalculateAndSave 计算并保存答卷分数
	CalculateAndSave(ctx context.Context, answerSheetID uint64) error
}

type answerSheetScoringService struct {
	answerSheetRepo   answersheet.Repository
	questionnaireRepo questionnaire.Repository
	answerScorer      ruleengine.AnswerScorer
}

// NewAnswerSheetScoringService 创建答卷计分应用服务
func NewAnswerSheetScoringService(
	answerSheetRepo answersheet.Repository,
	questionnaireRepo questionnaire.Repository,
	answerScorer ruleengine.AnswerScorer,
) AnswerSheetScoringService {
	return &answerSheetScoringService{
		answerSheetRepo:   answerSheetRepo,
		questionnaireRepo: questionnaireRepo,
		answerScorer:      answerScorer,
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
	sheetID, err := answerSheetIDFromUint64("answersheet_id", answerSheetID)
	if err != nil {
		return err
	}
	sheet, err := s.answerSheetRepo.FindByID(ctx, sheetID)
	if err != nil {
		l.Errorw("加载答卷失败", "answersheet_id", answerSheetID, "error", err.Error())
		return errors.WrapC(err, errorCode.ErrAnswerSheetNotFound, "答卷不存在")
	}

	l.Debugw("答卷加载成功", "answersheet_id", answerSheetID, "questionnaire_code", sheet.QuestionnaireRef().Code(), "answer_count", len(sheet.Answers()))

	// 2. 加载问卷精确版本
	qCode, qVersion, _ := sheet.QuestionnaireInfo()
	qnr, err := s.questionnaireRepo.FindByCodeVersion(ctx, qCode, qVersion)
	if err != nil {
		l.Errorw("加载问卷失败", "questionnaire_code", qCode, "questionnaire_version", qVersion, "error", err.Error())
		return errors.WrapC(err, errorCode.ErrQuestionnaireNotFound, "问卷不存在")
	}
	if qnr == nil {
		return errors.WithCode(errorCode.ErrQuestionnaireNotFound, "问卷不存在或版本不匹配")
	}

	l.Debugw("问卷加载成功", "questionnaire_code", qnr.GetCode().Value(), "questionnaire_version", qnr.GetVersion().Value(), "question_count", len(qnr.GetQuestions()))

	// 3. 计算分数
	scoredSheet, err := s.calculateAnswerSheetScore(ctx, sheet, qnr)
	if err != nil {
		l.Errorw("计算分数失败", "answersheet_id", answerSheetID, "error", err.Error())
		return errors.WrapC(err, errorCode.ErrAnswerSheetScoreCalculationFailed, "计算分数失败")
	}

	l.Debugw("分数计算完成", "answersheet_id", answerSheetID, "total_score", scoredSheet.TotalScore, "scored_answer_count", len(scoredSheet.ScoredAnswers))

	logZeroScoreDetails(l, sheet, qnr, scoredSheet)

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

func (s *answerSheetScoringService) calculateAnswerSheetScore(ctx context.Context, sheet *answersheet.AnswerSheet, qnr *questionnaire.Questionnaire) (*answersheet.ScoredAnswerSheet, error) {
	if s.answerScorer == nil {
		return nil, errors.WithCode(errorCode.ErrAnswerSheetScoreCalculationFailed, "答卷计分器未配置")
	}
	tasks := buildAnswerScoreTasks(sheet, qnr)
	results, err := s.answerScorer.ScoreAnswers(ctx, tasks)
	if err != nil {
		return nil, err
	}
	return scoredAnswerSheetFromResults(sheet, results), nil
}
