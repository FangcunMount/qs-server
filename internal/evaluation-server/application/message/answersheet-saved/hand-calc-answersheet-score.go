package answersheet_saved

import (
	"context"
	"encoding/json"
	"time"

	"github.com/yshujie/questionnaire-scale/internal/evaluation-server/domain/calculation"
	"github.com/yshujie/questionnaire-scale/internal/evaluation-server/domain/calculation/rules"
	grpcclient "github.com/yshujie/questionnaire-scale/internal/evaluation-server/infrastructure/grpc"
	"github.com/yshujie/questionnaire-scale/internal/pkg/pubsub"
	"github.com/yshujie/questionnaire-scale/pkg/log"

	answersheetpb "github.com/yshujie/questionnaire-scale/internal/apiserver/interface/grpc/proto/answersheet"
	questionnairepb "github.com/yshujie/questionnaire-scale/internal/apiserver/interface/grpc/proto/questionnaire"
)

// HandlerCalcAnswersheetScore 计算答卷分数处理器
type CalcAnswersheetScoreHandler struct {
	questionnaireClient *grpcclient.QuestionnaireClient
	answersheetClient   *grpcclient.AnswerSheetClient
	calculationEngine   *calculation.CalculationEngine
	maxConcurrency      int // 最大并发数
}

// NewCalcAnswersheetScoreHandler 创建计算答卷分数处理器
func NewCalcAnswersheetScoreHandler(
	questionnaireClient *grpcclient.QuestionnaireClient,
	answersheetClient *grpcclient.AnswerSheetClient,
) *CalcAnswersheetScoreHandler {
	return &CalcAnswersheetScoreHandler{
		questionnaireClient: questionnaireClient,
		answersheetClient:   answersheetClient,
		calculationEngine:   calculation.GetGlobalCalculationEngine(),
		maxConcurrency:      50, // 默认最大并发数为50
	}
}

// NewCalcAnswersheetScoreHandlerWithConcurrency 创建计算答卷分数处理器（带并发控制）
func NewCalcAnswersheetScoreHandlerWithConcurrency(
	questionnaireClient *grpcclient.QuestionnaireClient,
	answersheetClient *grpcclient.AnswerSheetClient,
	maxConcurrency int,
) *CalcAnswersheetScoreHandler {
	if maxConcurrency <= 0 {
		maxConcurrency = 50 // 默认值
	}
	return &CalcAnswersheetScoreHandler{
		questionnaireClient: questionnaireClient,
		answersheetClient:   answersheetClient,
		calculationEngine:   calculation.GetGlobalCalculationEngine(),
		maxConcurrency:      maxConcurrency,
	}
}

// Handle 计算答卷得分，并保存分数
func (h *CalcAnswersheetScoreHandler) Handle(ctx context.Context, data pubsub.AnswersheetSavedData) error {
	startTime := time.Now()
	log.Debugf("in HandlerCalcAnswersheetScore: %s", data)

	// 先加载答卷
	answersheet, err := h.loadAnswersheet(ctx, data.AnswerSheetID)
	if err != nil {
		return err
	}

	// 从答卷中获取问卷代码，然后加载问卷
	questionnaire, err := h.loadQuestionnaire(ctx, data.QuestionnaireCode, data.QuestionnaireVersion)
	if err != nil {
		return err
	}

	// 计算答卷中每一个答案的得分
	scoreStartTime := time.Now()
	if err := h.calculateAnswerScores(ctx, answersheet, questionnaire); err != nil {
		log.Errorf("计算答案得分失败: %v", err)
		return err
	}
	scoreDuration := time.Since(scoreStartTime)

	// 计算答卷总分
	totalStartTime := time.Now()
	if err := h.calculateAnswerSheetTotalScore(answersheet); err != nil {
		log.Errorf("计算答卷总分失败: %v", err)
		return err
	}
	totalScoreDuration := time.Since(totalStartTime)

	// 保存答卷得分
	saveStartTime := time.Now()
	if err := h.saveAnswerSheetScores(ctx, data.AnswerSheetID, answersheet); err != nil {
		log.Errorf("保存答卷得分失败: %v", err)
		return err
	}
	saveDuration := time.Since(saveStartTime)

	totalDuration := time.Since(startTime)
	log.Infof("答卷得分计算完成，答卷ID: %d, 总分: %d, 总耗时: %v (答案计算: %v, 总分计算: %v, 保存: %v)",
		data.AnswerSheetID, answersheet.Score, totalDuration, scoreDuration, totalScoreDuration, saveDuration)
	return nil
}

// loadQuestionnaire 加载问卷
func (h *CalcAnswersheetScoreHandler) loadQuestionnaire(ctx context.Context, questionnaireCode string, questionnaireVersion string) (*questionnairepb.Questionnaire, error) {
	loadedQuestionnaire, err := h.questionnaireClient.GetQuestionnaire(ctx, questionnaireCode)
	if err != nil {
		return nil, err
	}

	log.Debugf("loaded questionnaire: %s", loadedQuestionnaire.String())
	return loadedQuestionnaire, nil
}

// loadAnswersheet 加载答卷
func (h *CalcAnswersheetScoreHandler) loadAnswersheet(ctx context.Context, answerSheetID uint64) (*answersheetpb.AnswerSheet, error) {
	loadedAnswersheet, err := h.answersheetClient.GetAnswerSheet(ctx, answerSheetID)
	if err != nil {
		return nil, err
	}

	log.Debugf("loaded answersheet: %s", loadedAnswersheet.String())
	return loadedAnswersheet, nil
}

// calculateAnswerScores 计算答案得分
func (h *CalcAnswersheetScoreHandler) calculateAnswerScores(ctx context.Context, answersheet *answersheetpb.AnswerSheet, questionnaire *questionnairepb.Questionnaire) error {
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
	for i := 0; i < h.maxConcurrency; i++ {
		go func(workerID int) {
			for answer := range taskChan {
				result := answerResult{answer: answer}

				// 是否可以计算得分
				if !h.canCalculateScore(answer.QuestionCode, questionnaire) {
					resultChan <- result
					continue
				}

				// 获取计算公式
				formulaType := h.getCalculationFormulaType(answer.QuestionCode, questionnaire)

				// 获取计算操作数（根据问题的 option 和 答案选中值）
				operands := h.loadCalculationOperands(answer.QuestionCode, answer.Value, questionnaire)
				if len(operands) == 0 {
					resultChan <- result
					continue
				}

				// 创建计算规则
				rule := h.createCalculationRule(formulaType)

				// 执行计算
				calcResult, err := h.calculationEngine.Calculate(ctx, operands, rule)
				if err != nil {
					result.err = err
					resultChan <- result
					continue
				}

				// 保存计算结果
				result.score = uint32(calcResult.Value)
				resultChan <- result

				log.Debugf("Worker %d: 问题 %s 得分计算完成: %d", workerID, answer.QuestionCode, result.score)
			}
		}(i)
	}

	// 发送任务到任务通道
	for _, answer := range answersheet.Answers {
		taskChan <- answer
	}
	close(taskChan) // 关闭任务通道，通知 workers 没有更多任务

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
		len(answersheet.Answers), successCount, errorCount, h.maxConcurrency)
	return nil
}

// canCalculateScore 是否可以计算得分
// 判断 question 是否拥有 CalculationRule、CalculationRule.FormulaType 不为空
func (h *CalcAnswersheetScoreHandler) canCalculateScore(questionCode string, questionnaire *questionnairepb.Questionnaire) bool {
	question := findQuestionByCode(questionCode, questionnaire)
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
func (h *CalcAnswersheetScoreHandler) getCalculationFormulaType(questionCode string, questionnaire *questionnairepb.Questionnaire) string {
	// 获取问题
	question := findQuestionByCode(questionCode, questionnaire)
	if question == nil {
		log.Errorf("question not found: %s", questionCode)
		return ""
	}

	// 获取计算公式类型
	return question.CalculationRule.FormulaType
}

// createCalculationRule 创建计算规则
func (h *CalcAnswersheetScoreHandler) createCalculationRule(formulaType string) *rules.CalculationRule {
	// 将旧的公式类型映射到新的策略名称
	strategyName := h.mapFormulaTypeToStrategy(formulaType)
	return rules.NewCalculationRule(strategyName)
}

// mapFormulaTypeToStrategy 映射公式类型到策略名称
func (h *CalcAnswersheetScoreHandler) mapFormulaTypeToStrategy(formulaType string) string {
	switch formulaType {
	case "the_option", "score":
		return "option"
	case "sum":
		return "sum"
	case "average":
		return "average"
	case "max":
		return "max"
	case "min":
		return "min"
	default:
		return "option" // 默认使用选项策略
	}
}

// loadCalculationOperands 获取计算操作数
func (h *CalcAnswersheetScoreHandler) loadCalculationOperands(questionCode string, answerValue string, questionnaire *questionnairepb.Questionnaire) []float64 {
	// 获取问题
	question := findQuestionByCode(questionCode, questionnaire)
	if question == nil {
		log.Errorf("question not found: %s", questionCode)
		return nil
	}

	// 解析答案值（可能是JSON字符串）
	var actualValue string
	if err := json.Unmarshal([]byte(answerValue), &actualValue); err != nil {
		// 如果不是JSON格式，直接使用原值
		actualValue = answerValue
	}

	log.Debugf("解析答案值: 原始值=%s, 解析后=%s", answerValue, actualValue)

	// 遍历问题选项
	for _, option := range question.Options {
		if option.Code == actualValue {
			operands := []float64{float64(option.Score)}
			log.Debugf("找到匹配选项: %s, 得分: %d", option.Code, option.Score)
			return operands
		}
	}

	log.Warnf("未找到匹配的选项: 问题=%s, 答案值=%s", questionCode, actualValue)
	return nil
}

// calculateAnswerSheetTotalScore 计算答卷总分
func (h *CalcAnswersheetScoreHandler) calculateAnswerSheetTotalScore(answersheet *answersheetpb.AnswerSheet) error {
	// 使用 goroutine 并发计算总分
	type totalResult struct {
		totalScore float64
		err        error
	}

	// 创建结果通道
	resultChan := make(chan totalResult, 1)

	// 启动 goroutine 计算总分
	go func() {
		var totalScore float64
		// 遍历答卷中的每个答案
		for _, answer := range answersheet.Answers {
			totalScore += float64(answer.Score)
		}

		resultChan <- totalResult{totalScore: totalScore}
	}()

	// 等待计算结果
	result := <-resultChan
	if result.err != nil {
		return result.err
	}

	answersheet.Score = uint32(result.totalScore)
	log.Debugf("答卷总分计算完成: %d", answersheet.Score)
	return nil
}

// saveAnswerSheetScores 保存答卷得分
func (h *CalcAnswersheetScoreHandler) saveAnswerSheetScores(ctx context.Context, answerSheetID uint64, answersheet *answersheetpb.AnswerSheet) error {
	// 保存答卷得分
	err := h.answersheetClient.SaveAnswerSheetScores(ctx, answerSheetID, answersheet.Score, answersheet.Answers)
	if err != nil {
		return err
	}

	log.Debugf("answersheet score saved: %d", answersheet.Score)
	return nil
}

// findQuestionByCode 根据问题代码查找问题
func findQuestionByCode(questionCode string, questionnaire *questionnairepb.Questionnaire) *questionnairepb.Question {
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
