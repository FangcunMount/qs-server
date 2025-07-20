package calculation

import (
	"context"
	"encoding/json"

	answersheetpb "github.com/yshujie/questionnaire-scale/internal/apiserver/interface/grpc/proto/answersheet"
	questionnairepb "github.com/yshujie/questionnaire-scale/internal/apiserver/interface/grpc/proto/questionnaire"
	"github.com/yshujie/questionnaire-scale/internal/evaluation-server/domain/calculation"
	"github.com/yshujie/questionnaire-scale/pkg/log"
)

// ScoreCalculator 分数计算器接口
type ScoreCalculator interface {
	CalculateAnswerScores(ctx context.Context, answersheet *answersheetpb.AnswerSheet, questionnaire *questionnairepb.Questionnaire) error
	CalculateTotalScore(ctx context.Context, answersheet *answersheetpb.AnswerSheet) error
}

// CalculatorFactory 计算器工厂接口
type CalculatorFactory interface {
	GetCalculator(calculatorType calculation.CalculaterType) (calculation.Calculater, error)
}

// scoreCalculator 分数计算器实现
type scoreCalculator struct {
	calculatorFactory CalculatorFactory
	maxConcurrency    int
}

// NewScoreCalculator 创建分数计算器
func NewScoreCalculator(calculatorFactory CalculatorFactory, maxConcurrency int) ScoreCalculator {
	if maxConcurrency <= 0 {
		maxConcurrency = 50
	}
	return &scoreCalculator{
		calculatorFactory: calculatorFactory,
		maxConcurrency:    maxConcurrency,
	}
}

// CalculateAnswerScores 计算答案分数
func (s *scoreCalculator) CalculateAnswerScores(ctx context.Context, answersheet *answersheetpb.AnswerSheet, questionnaire *questionnairepb.Questionnaire) error {
	// 使用 worker pool 模式并发计算每个答案的得分
	type answerResult struct {
		answer *answersheetpb.Answer
		score  uint32
		err    error
	}

	// 创建任务通道和结果通道
	taskChan := make(chan *answersheetpb.Answer, len(answersheet.Answers))
	resultChan := make(chan answerResult, len(answersheet.Answers))

	// 启动 worker goroutines
	for i := 0; i < s.maxConcurrency; i++ {
		go func(workerID int) {
			for answer := range taskChan {
				result := answerResult{answer: answer}

				// 是否可以计算得分
				if !s.canCalculateScore(answer.QuestionCode, questionnaire) {
					resultChan <- result
					continue
				}

				// 获取计算公式
				formulaType := s.getCalculationFormulaType(answer.QuestionCode, questionnaire)

				// 根据计算规则，创建计算器
				calculator, err := s.calculatorFactory.GetCalculator(calculation.CalculaterType(formulaType))
				if err != nil {
					result.err = err
					resultChan <- result
					continue
				}

				// 获取计算操作数
				operands := s.loadCalculationOperands(answer.QuestionCode, answer.Value, questionnaire)

				// 执行计算
				score, err := calculator.Calculate(operands)
				if err != nil {
					result.err = err
					resultChan <- result
					continue
				}

				// 保存计算结果
				result.score = uint32(score)
				resultChan <- result

				log.Debugf("Worker %d: 问题 %s 得分计算完成: %d", workerID, answer.QuestionCode, result.score)
			}
		}(i)
	}

	// 发送任务到任务通道
	for _, answer := range answersheet.Answers {
		taskChan <- answer
	}
	close(taskChan)

	// 收集所有计算结果
	completedCount := 0
	errorCount := 0
	successCount := 0

	for completedCount < len(answersheet.Answers) {
		result := <-resultChan
		completedCount++

		if result.err != nil {
			errorCount++
			log.Errorf("计算答案得分失败，问题代码: %s, 错误: %v", result.answer.QuestionCode, result.err)
			continue
		}

		// 更新答案的得分
		result.answer.Score = result.score
		successCount++
	}

	log.Infof("所有答案得分计算完成，共处理 %d 个答案，成功 %d 个，失败 %d 个，使用 %d 个 worker",
		len(answersheet.Answers), successCount, errorCount, s.maxConcurrency)
	return nil
}

// CalculateTotalScore 计算总分
func (s *scoreCalculator) CalculateTotalScore(ctx context.Context, answersheet *answersheetpb.AnswerSheet) error {
	var totalScore float64
	for _, answer := range answersheet.Answers {
		totalScore += float64(answer.Score)
	}

	answersheet.Score = uint32(totalScore)
	log.Debugf("答卷总分计算完成: %d", answersheet.Score)
	return nil
}

// canCalculateScore 是否可以计算得分
func (s *scoreCalculator) canCalculateScore(questionCode string, questionnaire *questionnairepb.Questionnaire) bool {
	question := s.findQuestionByCode(questionCode, questionnaire)
	if question == nil {
		log.Debugf("question not found: %s", questionCode)
		return false
	}

	if question.CalculationRule == nil {
		log.Debugf("question calculation rule not found: %s", question.Title)
		return false
	}

	if question.CalculationRule.FormulaType == "" {
		log.Debugf("question calculation rule formula type is empty: %s", question.Title)
		return false
	}

	return true
}

// getCalculationFormulaType 获取计算公式类型
func (s *scoreCalculator) getCalculationFormulaType(questionCode string, questionnaire *questionnairepb.Questionnaire) string {
	question := s.findQuestionByCode(questionCode, questionnaire)
	if question == nil {
		log.Errorf("question not found: %s", questionCode)
		return ""
	}
	return question.CalculationRule.FormulaType
}

// loadCalculationOperands 获取计算操作数
func (s *scoreCalculator) loadCalculationOperands(questionCode string, answerValue string, questionnaire *questionnairepb.Questionnaire) []calculation.Operand {
	var operands []calculation.Operand

	question := s.findQuestionByCode(questionCode, questionnaire)
	if question == nil {
		log.Errorf("question not found: %s", questionCode)
		return operands
	}

	// 解析答案值
	var actualValue string
	if err := json.Unmarshal([]byte(answerValue), &actualValue); err != nil {
		actualValue = answerValue
	}

	log.Debugf("解析答案值: 原始值=%s, 解析后=%s", answerValue, actualValue)

	// 遍历问题选项
	for _, option := range question.Options {
		if option.Code == actualValue {
			operands = append(operands, calculation.Operand(option.Score))
			log.Debugf("找到匹配选项: %s, 得分: %d", option.Code, option.Score)
			break
		}
	}

	if len(operands) == 0 {
		log.Warnf("未找到匹配的选项: 问题=%s, 答案值=%s", questionCode, actualValue)
	}

	return operands
}

// findQuestionByCode 根据问题代码查找问题
func (s *scoreCalculator) findQuestionByCode(questionCode string, questionnaire *questionnairepb.Questionnaire) *questionnairepb.Question {
	if questionnaire == nil {
		log.Errorf("questionnaire is nil, cannot find question: %s", questionCode)
		return nil
	}

	for _, question := range questionnaire.Questions {
		if question.Code == questionCode {
			return question
		}
	}
	return nil
}
