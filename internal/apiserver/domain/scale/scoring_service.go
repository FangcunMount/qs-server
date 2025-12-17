package scale

import (
	"context"
	"fmt"

	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
)

// ScoringService 计分服务（领域服务）
// 职责：根据量表的计分配置，计算因子得分
type ScoringService interface {
	// CalculateFactorScore 计算因子得分
	CalculateFactorScore(
		ctx context.Context,
		factor *Factor,
		sheet *answersheet.AnswerSheet,
		qnr *questionnaire.Questionnaire,
	) (float64, error)
}

// defaultScoringService 默认计分服务实现
type defaultScoringService struct{}

// NewScoringService 创建计分服务
func NewScoringService() ScoringService {
	return &defaultScoringService{}
}

// CalculateFactorScore 计算因子得分
func (s *defaultScoringService) CalculateFactorScore(
	ctx context.Context,
	factor *Factor,
	sheet *answersheet.AnswerSheet,
	qnr *questionnaire.Questionnaire,
) (float64, error) {
	l := logger.L(ctx)

	if factor == nil || sheet == nil {
		return 0, fmt.Errorf("factor and answer sheet are required")
	}

	questionCodes := factor.GetQuestionCodes()
	if len(questionCodes) == 0 {
		l.Debugw("Factor has no questions, returning score 0",
			"factor_code", factor.GetCode().Value())
		return 0, nil
	}

	l.Infow("Calculating factor score",
		"factor_code", factor.GetCode().Value(),
		"strategy", factor.GetScoringStrategy(),
		"question_count", len(questionCodes))

	// 严格根据配置的计分策略执行，不做降级
	var score float64
	var err error

	switch factor.GetScoringStrategy() {
	case ScoringStrategySum:
		score, err = s.applySumStrategy(ctx, factor, sheet)

	case ScoringStrategyAvg:
		score, err = s.applyAvgStrategy(ctx, factor, sheet)

	case ScoringStrategyCnt:
		// cnt 策略需要问卷信息和配置参数
		if qnr == nil {
			return 0, fmt.Errorf("questionnaire is required for cnt scoring strategy")
		}
		score, err = s.applyCntStrategy(ctx, factor, sheet, qnr)

	default:
		// 未知策略，报错而不是降级
		err = fmt.Errorf("unknown scoring strategy: %s", factor.GetScoringStrategy())
	}

	if err != nil {
		l.Errorw("Failed to calculate factor score",
			"factor_code", factor.GetCode().Value(),
			"strategy", factor.GetScoringStrategy(),
			"error", err)
		return 0, err
	}

	l.Infow("Factor score calculated successfully",
		"factor_code", factor.GetCode().Value(),
		"strategy", factor.GetScoringStrategy(),
		"score", score)

	return score, nil
}

// applyCntStrategy 应用计数策略
// 统计选择了特定选项内容的题目数量
func (s *defaultScoringService) applyCntStrategy(
	ctx context.Context,
	factor *Factor,
	sheet *answersheet.AnswerSheet,
	qnr *questionnaire.Questionnaire,
) (float64, error) {
	l := logger.L(ctx)

	// 从计分参数中获取计数策略的选项内容列表
	params := factor.GetScoringParams()
	targetContents := params.GetCntOptionContents()

	if len(targetContents) == 0 {
		return 0, fmt.Errorf("cnt_option_contents is empty")
	}

	l.Debugw("Applying cnt strategy",
		"factor_code", factor.GetCode().Value(),
		"target_contents", targetContents)

	// 构建选项内容映射
	optionContentMap := buildOptionContentMap(qnr)

	// 构建答案映射
	answerMap := buildAnswerMap(sheet)

	// 筛选出匹配的题目，收集为 1.0（匹配）或 0.0（不匹配）
	matchValues := make([]float64, 0, len(factor.GetQuestionCodes()))
	matchedQuestions := make([]string, 0)

	for _, qCode := range factor.GetQuestionCodes() {
		answer, found := answerMap[qCode.String()]
		if !found {
			l.Debugw("Question not answered", "question_code", qCode.String())
			continue
		}

		// 获取答案的选项ID
		optionID := extractOptionID(answer)
		if optionID == "" {
			l.Debugw("No option ID found in answer", "question_code", qCode.String())
			continue
		}

		// 获取选项内容
		optionContent, found := optionContentMap[optionID]
		if !found {
			l.Warnw("Option content not found",
				"question_code", qCode.String(),
				"option_id", optionID)
			continue
		}

		// 判断是否匹配目标内容
		if containsString(targetContents, optionContent) {
			matchValues = append(matchValues, 1.0) // 匹配
			matchedQuestions = append(matchedQuestions, qCode.String())
			l.Debugw("Question matched target content",
				"question_code", qCode.String(),
				"option_content", optionContent)
		}
	}

	l.Infow("Cnt strategy matching completed",
		"factor_code", factor.GetCode().Value(),
		"total_questions", len(factor.GetQuestionCodes()),
		"matched_count", len(matchValues),
		"matched_questions", matchedQuestions)

	// 使用 calculation 包的 Count 策略计数
	countStrategy := calculation.GetStrategy(calculation.StrategyTypeCount)
	if countStrategy == nil {
		// 降级：手动计数
		return float64(len(matchValues)), nil
	}

	return countStrategy.Calculate(matchValues, nil)
}

// applySumStrategy 应用求和策略
func (s *defaultScoringService) applySumStrategy(ctx context.Context, factor *Factor, sheet *answersheet.AnswerSheet) (float64, error) {
	l := logger.L(ctx)

	scores := s.collectQuestionScores(factor, sheet)
	if len(scores) == 0 {
		l.Debugw("No scores collected for sum strategy",
			"factor_code", factor.GetCode().Value())
		return 0, nil
	}

	l.Debugw("Applying sum strategy",
		"factor_code", factor.GetCode().Value(),
		"scores", scores,
		"score_count", len(scores))

	strategy := calculation.GetStrategy(calculation.StrategyTypeSum)
	if strategy == nil {
		// 手动求和作为降级
		sum := 0.0
		for _, score := range scores {
			sum += score
		}
		l.Debugw("Sum calculated (manual fallback)",
			"factor_code", factor.GetCode().Value(),
			"sum", sum)
		return sum, nil
	}

	result, err := strategy.Calculate(scores, nil)
	if err == nil {
		l.Debugw("Sum calculated",
			"factor_code", factor.GetCode().Value(),
			"sum", result)
	}
	return result, err
}

// applyAvgStrategy 应用平均值策略
func (s *defaultScoringService) applyAvgStrategy(ctx context.Context, factor *Factor, sheet *answersheet.AnswerSheet) (float64, error) {
	l := logger.L(ctx)

	scores := s.collectQuestionScores(factor, sheet)
	if len(scores) == 0 {
		l.Debugw("No scores collected for avg strategy",
			"factor_code", factor.GetCode().Value())
		return 0, nil
	}

	l.Debugw("Applying avg strategy",
		"factor_code", factor.GetCode().Value(),
		"scores", scores,
		"score_count", len(scores))

	strategy := calculation.GetStrategy(calculation.StrategyTypeAverage)
	if strategy == nil {
		// 手动计算平均值作为降级
		sum := 0.0
		for _, score := range scores {
			sum += score
		}
		avg := sum / float64(len(scores))
		l.Debugw("Average calculated (manual fallback)",
			"factor_code", factor.GetCode().Value(),
			"average", avg)
		return avg, nil
	}

	result, err := strategy.Calculate(scores, nil)
	if err == nil {
		l.Debugw("Average calculated",
			"factor_code", factor.GetCode().Value(),
			"average", result)
	}
	return result, err
}

// collectQuestionScores 收集因子关联题目的得分
func (s *defaultScoringService) collectQuestionScores(factor *Factor, sheet *answersheet.AnswerSheet) []float64 {
	answerMap := buildAnswerMap(sheet)
	scores := make([]float64, 0, len(factor.GetQuestionCodes()))

	for _, qCode := range factor.GetQuestionCodes() {
		if answer, found := answerMap[qCode.String()]; found {
			scores = append(scores, answer.Score())
		}
	}

	return scores
}

// ==================== 辅助函数 ====================

// buildOptionContentMap 构建选项ID到内容的映射
func buildOptionContentMap(qnr *questionnaire.Questionnaire) map[string]string {
	contentMap := make(map[string]string)

	for _, q := range qnr.GetQuestions() {
		for _, opt := range q.GetOptions() {
			contentMap[opt.GetCode().Value()] = opt.GetContent()
		}
	}

	return contentMap
}

// buildAnswerMap 构建答案映射
func buildAnswerMap(sheet *answersheet.AnswerSheet) map[string]answersheet.Answer {
	answerMap := make(map[string]answersheet.Answer)
	for _, ans := range sheet.Answers() {
		answerMap[ans.QuestionCode()] = ans
	}
	return answerMap
}

// extractOptionID 从答案中提取选项ID
func extractOptionID(answer answersheet.Answer) string {
	value := answer.Value()
	if value == nil {
		return ""
	}

	raw := value.Raw()
	if raw == nil {
		return ""
	}

	// 处理单选：string
	if str, ok := raw.(string); ok {
		return str
	}

	// 处理多选：[]string，取第一个
	if arr, ok := raw.([]string); ok && len(arr) > 0 {
		return arr[0]
	}

	return ""
}

// containsString 判断字符串数组是否包含指定字符串
func containsString(arr []string, target string) bool {
	for _, s := range arr {
		if s == target {
			return true
		}
	}
	return false
}
