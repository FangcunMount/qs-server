package answersheet_saved

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	answersheetpb "github.com/fangcun-mount/qs-server/internal/apiserver/interface/grpc/proto/answersheet"
	questionnairepb "github.com/fangcun-mount/qs-server/internal/apiserver/interface/grpc/proto/questionnaire"
	calculationapp "github.com/fangcun-mount/qs-server/internal/evaluation-server/application/calculation"
	grpcclient "github.com/fangcun-mount/qs-server/internal/evaluation-server/infrastructure/grpc"
	"github.com/fangcun-mount/qs-server/internal/pkg/pubsub"
	"github.com/fangcun-mount/qs-server/pkg/log"
)

// CalcAnswersheetScoreHandler 计算答卷分数处理器
type CalcAnswersheetScoreHandler struct {
	questionnaireClient *grpcclient.QuestionnaireClient
	answersheetClient   *grpcclient.AnswerSheetClient
	calculationPort     calculationapp.CalculationPort
}

// NewCalcAnswersheetScoreHandler 创建计算答卷分数处理器（默认并发）
func NewCalcAnswersheetScoreHandler(
	questionnaireClient *grpcclient.QuestionnaireClient,
	answersheetClient *grpcclient.AnswerSheetClient,
) *CalcAnswersheetScoreHandler {
	return &CalcAnswersheetScoreHandler{
		questionnaireClient: questionnaireClient,
		answersheetClient:   answersheetClient,
		calculationPort:     calculationapp.GetConcurrentCalculationPort(50), // 默认并发50
	}
}

// NewCalcAnswersheetScoreHandlerWithAdapter 创建计算答卷分数处理器（自定义适配器）
func NewCalcAnswersheetScoreHandlerWithAdapter(
	questionnaireClient *grpcclient.QuestionnaireClient,
	answersheetClient *grpcclient.AnswerSheetClient,
	calculationPort calculationapp.CalculationPort,
) *CalcAnswersheetScoreHandler {
	return &CalcAnswersheetScoreHandler{
		questionnaireClient: questionnaireClient,
		answersheetClient:   answersheetClient,
		calculationPort:     calculationPort,
	}
}

// Handle 计算答卷得分，并保存分数
func (h *CalcAnswersheetScoreHandler) Handle(ctx context.Context, data pubsub.AnswersheetSavedData) error {
	startTime := time.Now()
	log.Debugf("开始计算答卷分数: %s", data)

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

	// 计算答案分数
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

// calculateAnswerScores 计算答案分数（业务逻辑层）
func (h *CalcAnswersheetScoreHandler) calculateAnswerScores(ctx context.Context, answersheet *answersheetpb.AnswerSheet, questionnaire *questionnairepb.Questionnaire) error {
	log.Infof("开始计算答案分数，答案数量: %d", len(answersheet.Answers))

	// 转换为计算请求
	requests, err := h.convertAnswerBatchCalculation(answersheet, questionnaire)
	if err != nil {
		return err
	}

	if len(requests) == 0 {
		log.Infof("没有需要计算的答案")
		return nil
	}

	// 使用计算端口进行批量计算
	results, err := h.calculationPort.CalculateBatch(ctx, requests)
	if err != nil {
		return err
	}

	// 应用计算结果到答案
	return h.applyAnswerCalculationResults(answersheet, results)
}

// applyAnswerCalculationResults 应用答案计算结果
func (h *CalcAnswersheetScoreHandler) applyAnswerCalculationResults(answersheet *answersheetpb.AnswerSheet, results []*calculationapp.CalculationResult) error {
	answerMap := make(map[string]*answersheetpb.Answer)
	for _, answer := range answersheet.Answers {
		answerMap[answer.QuestionCode] = answer
	}

	successCount := 0
	errorCount := 0

	for _, result := range results {
		if result.Error != "" {
			errorCount++
			log.Errorf("计算失败，任务: %s, 错误: %s", result.Name, result.Error)
			continue
		}

		// 从结果ID提取问题代码
		if questionCode := extractQuestionCodeFromResultID(result.ID); questionCode != "" {
			if answer, exists := answerMap[questionCode]; exists {
				answer.Score = uint32(result.Value)
				successCount++
				log.Debugf("问题 %s 得分更新: %d", questionCode, answer.Score)
			}
		}
	}

	log.Infof("答案得分计算完成，成功 %d 个，失败 %d 个", successCount, errorCount)
	return nil
}

// extractQuestionCodeFromResultID 从结果ID提取问题代码
func extractQuestionCodeFromResultID(resultID string) string {
	const prefix = "answer_"
	if len(resultID) > len(prefix) && resultID[:len(prefix)] == prefix {
		return resultID[len(prefix):]
	}
	return ""
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

// calculateAnswerSheetTotalScore 计算答卷总分
func (h *CalcAnswersheetScoreHandler) calculateAnswerSheetTotalScore(answersheet *answersheetpb.AnswerSheet) error {
	var totalScore float64
	for _, answer := range answersheet.Answers {
		totalScore += float64(answer.Score)
	}

	answersheet.Score = float64(totalScore)
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

// SetCalculationPort 设置计算端口（运行时切换适配器）
func (h *CalcAnswersheetScoreHandler) SetCalculationPort(port calculationapp.CalculationPort) {
	h.calculationPort = port
}

// convertAnswerBatchCalculation 批量转换答案计算请求（私有方法）
func (h *CalcAnswersheetScoreHandler) convertAnswerBatchCalculation(answersheet *answersheetpb.AnswerSheet, questionnaire *questionnairepb.Questionnaire) ([]*calculationapp.CalculationRequest, error) {
	if answersheet == nil || questionnaire == nil {
		return nil, fmt.Errorf("答卷或问卷不能为空")
	}

	// 创建问题映射
	questionMap := make(map[string]*questionnairepb.Question)
	for _, question := range questionnaire.Questions {
		questionMap[question.Code] = question
	}

	var requests []*calculationapp.CalculationRequest

	for _, answer := range answersheet.Answers {
		question, exists := questionMap[answer.QuestionCode]
		if !exists {
			log.Warnf("未找到答案对应的问题: %s", answer.QuestionCode)
			continue
		}

		// 检查是否需要计算
		if question.CalculationRule == nil || question.CalculationRule.FormulaType == "" {
			log.Debugf("问题 %s 无需计算", answer.QuestionCode)
			continue
		}

		request, err := h.convertAnswerCalculation(answer, question)
		if err != nil {
			log.Errorf("转换答案计算请求失败，问题: %s, 错误: %v", answer.QuestionCode, err)
			continue
		}

		requests = append(requests, request)
	}

	log.Infof("批量转换答案计算请求完成，共生成 %d 个计算任务", len(requests))
	return requests, nil
}

// convertAnswerCalculation 转换答案计算请求（私有方法）
func (h *CalcAnswersheetScoreHandler) convertAnswerCalculation(answer *answersheetpb.Answer, question *questionnairepb.Question) (*calculationapp.CalculationRequest, error) {
	if answer == nil || question == nil {
		return nil, fmt.Errorf("答案或问题不能为空")
	}

	if question.CalculationRule == nil || question.CalculationRule.FormulaType == "" {
		return nil, fmt.Errorf("问题 %s 没有有效的计算规则", question.Code)
	}

	// 解析答案值获取操作数
	operands, err := h.extractOperandsFromAnswer(answer, question)
	if err != nil {
		return nil, fmt.Errorf("解析答案操作数失败: %w", err)
	}

	return &calculationapp.CalculationRequest{
		ID:          fmt.Sprintf("answer_%s", answer.QuestionCode),
		Name:        fmt.Sprintf("问题 %s 答案计算", question.Title),
		FormulaType: question.CalculationRule.FormulaType,
		Operands:    operands,
		Parameters: map[string]interface{}{
			"question_code": answer.QuestionCode,
			"question_type": answer.QuestionType,
			"answer_value":  answer.Value,
		},
		Precision:    2,
		RoundingMode: "round",
	}, nil
}

// extractOperandsFromAnswer 从答案中提取操作数（私有方法）
func (h *CalcAnswersheetScoreHandler) extractOperandsFromAnswer(answer *answersheetpb.Answer, question *questionnairepb.Question) ([]float64, error) {
	// 解析答案值
	var actualValue string
	if err := json.Unmarshal([]byte(answer.Value), &actualValue); err != nil {
		// 如果不是JSON格式，直接使用原值
		actualValue = answer.Value
	}

	log.Debugf("解析答案值: 原始值=%s, 解析后=%s", answer.Value, actualValue)

	// 遍历问题选项寻找匹配的得分
	for _, option := range question.Options {
		if option.Code == actualValue {
			operands := []float64{float64(option.Score)}
			log.Debugf("找到匹配选项: %s, 得分: %d", option.Code, option.Score)
			return operands, nil
		}
	}

	return nil, fmt.Errorf("未找到匹配的选项: 问题=%s, 答案值=%s", question.Code, actualValue)
}
