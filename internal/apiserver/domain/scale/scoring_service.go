package scale

import (
	"encoding/json"
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/pkg/calculation"
)

// ScoringService 计分服务（领域服务）
// 职责：根据量表的计分配置，计算因子得分
type ScoringService interface {
	// CalculateFactorScore 计算因子得分
	CalculateFactorScore(
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
	factor *Factor,
	sheet *answersheet.AnswerSheet,
	qnr *questionnaire.Questionnaire,
) (float64, error) {
	if factor == nil || sheet == nil {
		return 0, fmt.Errorf("factor and answer sheet are required")
	}

	questionCodes := factor.GetQuestionCodes()
	if len(questionCodes) == 0 {
		return 0, nil
	}

	// 严格根据配置的计分策略执行，不做降级
	switch factor.GetScoringStrategy() {
	case ScoringStrategySum:
		return s.applySumStrategy(factor, sheet)

	case ScoringStrategyAvg:
		return s.applyAvgStrategy(factor, sheet)

	case ScoringStrategyCnt:
		// cnt 策略需要问卷信息和配置参数
		if qnr == nil {
			return 0, fmt.Errorf("questionnaire is required for cnt scoring strategy")
		}
		return s.applyCntStrategy(factor, sheet, qnr)

	default:
		// 未知策略，报错而不是降级
		return 0, fmt.Errorf("unknown scoring strategy: %s", factor.GetScoringStrategy())
	}
}

// applyCntStrategy 应用计数策略
// 统计选择了特定选项内容的题目数量
func (s *defaultScoringService) applyCntStrategy(
	factor *Factor,
	sheet *answersheet.AnswerSheet,
	qnr *questionnaire.Questionnaire,
) (float64, error) {
	// 从 raw_calc_rule 中提取参数
	params := factor.GetScoringParams()
	rawRule, exists := params["raw_calc_rule"]
	if !exists || rawRule == "" {
		return 0, fmt.Errorf("raw_calc_rule is required for cnt scoring strategy")
	}

	// 解析计分规则，提取 AppendParams
	var rule struct {
		AppendParams map[string]interface{} `json:"AppendParams"`
	}
	if err := json.Unmarshal([]byte(rawRule), &rule); err != nil {
		return 0, fmt.Errorf("failed to parse raw_calc_rule: %w", err)
	}

	appendParams := rule.AppendParams
	// 获取目标选项内容列表
	targetContents, err := extractTargetContents(appendParams)
	if err != nil {
		return 0, err
	}

	if len(targetContents) == 0 {
		return 0, fmt.Errorf("cnt_option_contents is empty")
	}

	// 构建选项内容映射
	optionContentMap := buildOptionContentMap(qnr)

	// 构建答案映射
	answerMap := buildAnswerMap(sheet)

	// 筛选出匹配的题目，收集为 1.0（匹配）或 0.0（不匹配）
	matchValues := make([]float64, 0, len(factor.GetQuestionCodes()))
	for _, qCode := range factor.GetQuestionCodes() {
		answer, found := answerMap[qCode.String()]
		if !found {
			continue
		}

		// 获取答案的选项ID
		optionID := extractOptionID(answer)
		if optionID == "" {
			continue
		}

		// 获取选项内容
		optionContent, found := optionContentMap[optionID]
		if !found {
			continue
		}

		// 判断是否匹配目标内容
		if containsString(targetContents, optionContent) {
			matchValues = append(matchValues, 1.0) // 匹配
		}
	}

	// 使用 calculation 包的 Count 策略计数
	countStrategy := calculation.GetStrategy(calculation.StrategyTypeCount)
	if countStrategy == nil {
		// 降级：手动计数
		return float64(len(matchValues)), nil
	}

	return countStrategy.Calculate(matchValues, nil)
}

// applySumStrategy 应用求和策略
func (s *defaultScoringService) applySumStrategy(factor *Factor, sheet *answersheet.AnswerSheet) (float64, error) {
	scores := s.collectQuestionScores(factor, sheet)
	if len(scores) == 0 {
		return 0, nil
	}

	strategy := calculation.GetStrategy(calculation.StrategyTypeSum)
	if strategy == nil {
		// 手动求和作为降级
		sum := 0.0
		for _, score := range scores {
			sum += score
		}
		return sum, nil
	}

	return strategy.Calculate(scores, nil)
}

// applyAvgStrategy 应用平均值策略
func (s *defaultScoringService) applyAvgStrategy(factor *Factor, sheet *answersheet.AnswerSheet) (float64, error) {
	scores := s.collectQuestionScores(factor, sheet)
	if len(scores) == 0 {
		return 0, nil
	}

	strategy := calculation.GetStrategy(calculation.StrategyTypeAverage)
	if strategy == nil {
		// 手动计算平均值作为降级
		sum := 0.0
		for _, score := range scores {
			sum += score
		}
		return sum / float64(len(scores)), nil
	}

	return strategy.Calculate(scores, nil)
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

// extractTargetContents 从附加参数中提取目标选项内容列表
func extractTargetContents(appendParams map[string]interface{}) ([]string, error) {
	rawContents, exists := appendParams["cnt_option_contents"]
	if !exists {
		return nil, fmt.Errorf("cnt_option_contents not found in append params")
	}

	switch v := rawContents.(type) {
	case []interface{}:
		result := make([]string, 0, len(v))
		for _, item := range v {
			if str, ok := item.(string); ok {
				result = append(result, str)
			}
		}
		return result, nil
	case []string:
		return v, nil
	default:
		return nil, fmt.Errorf("cnt_option_contents has invalid type: %T", rawContents)
	}
}

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
