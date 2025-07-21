package answersheet_saved

import (
	"context"
	"fmt"
	"sync"

	answersheetpb "github.com/yshujie/questionnaire-scale/internal/apiserver/interface/grpc/proto/answersheet"
	interpretreportpb "github.com/yshujie/questionnaire-scale/internal/apiserver/interface/grpc/proto/interpret-report"
	medicalscalepb "github.com/yshujie/questionnaire-scale/internal/apiserver/interface/grpc/proto/medical-scale"
	"github.com/yshujie/questionnaire-scale/internal/evaluation-server/domain/calculation"
	"github.com/yshujie/questionnaire-scale/internal/evaluation-server/domain/calculation/rules"
	"github.com/yshujie/questionnaire-scale/internal/evaluation-server/domain/interpretion"
	grpcclient "github.com/yshujie/questionnaire-scale/internal/evaluation-server/infrastructure/grpc"
	"github.com/yshujie/questionnaire-scale/internal/pkg/pubsub"
	"github.com/yshujie/questionnaire-scale/pkg/log"
)

// GenerateInterpretReportHandlerConcurrent 并发版本的解读报告生成处理器
type GenerateInterpretReportHandlerConcurrent struct {
	answersheetClient     *grpcclient.AnswerSheetClient
	medicalScaleClient    *grpcclient.MedicalScaleClient
	interpretReportClient *grpcclient.InterpretReportClient
	calculationEngine     *calculation.CalculationEngine
	maxConcurrency        int
}

// NewGenerateInterpretReportHandlerConcurrent 创建并发版本的解读报告生成处理器
func NewGenerateInterpretReportHandlerConcurrent(
	answersheetClient *grpcclient.AnswerSheetClient,
	medicalScaleClient *grpcclient.MedicalScaleClient,
	interpretReportClient *grpcclient.InterpretReportClient,
	maxConcurrency int,
) *GenerateInterpretReportHandlerConcurrent {
	if maxConcurrency <= 0 {
		maxConcurrency = 10
	}
	return &GenerateInterpretReportHandlerConcurrent{
		answersheetClient:     answersheetClient,
		medicalScaleClient:    medicalScaleClient,
		interpretReportClient: interpretReportClient,
		calculationEngine:     calculation.GetGlobalCalculationEngine(),
		maxConcurrency:        maxConcurrency,
	}
}

// Handle 并发处理解读报告生成
func (h *GenerateInterpretReportHandlerConcurrent) Handle(ctx context.Context, data pubsub.AnswersheetSavedData) error {
	log.Infof("开始并发计算解读报告分数，答卷ID: %d, 问卷代码: %s, 最大并发数: %d", data.AnswerSheetID, data.QuestionnaireCode, h.maxConcurrency)

	// 加载答卷
	answerSheet, err := h.loadAnswerSheet(ctx, data.AnswerSheetID)
	if err != nil {
		log.Errorf("加载答卷失败，ID: %d, 错误: %v", data.AnswerSheetID, err)
		return fmt.Errorf("加载答卷失败: %w", err)
	}

	// 加载医学量表
	medicalScale, err := h.loadMedicalScale(ctx, data.QuestionnaireCode)
	if err != nil {
		log.Errorf("加载医学量表失败，代码: %s, 错误: %v", data.QuestionnaireCode, err)
		return fmt.Errorf("加载医学量表失败: %w", err)
	}

	// 创建解读报告
	interpretReport := &interpretreportpb.InterpretReport{
		Id:               data.AnswerSheetID,
		AnswerSheetId:    data.AnswerSheetID,
		MedicalScaleCode: data.QuestionnaireCode,
		Title:            medicalScale.Title,
		Description:      medicalScale.Description,
		InterpretItems:   h.buildInterpretItems(medicalScale),
	}

	// 并发计算解读报告中的因子分
	if err := h.calculateInterpretReportScoreConcurrent(ctx, interpretReport, answerSheet, medicalScale); err != nil {
		log.Errorf("计算解读报告分数失败，错误: %v", err)
		return fmt.Errorf("计算解读报告分数失败: %w", err)
	}

	// 生成解读内容
	contentGenerator := interpretion.NewInterpretReportContentGenerator()
	if err := contentGenerator.GenerateInterpretContent(interpretReport, medicalScale); err != nil {
		log.Errorf("生成解读内容失败，错误: %v", err)
		return fmt.Errorf("生成解读内容失败: %w", err)
	}

	// 保存解读报告
	if err := h.saveInterpretReport(ctx, interpretReport); err != nil {
		log.Errorf("保存解读报告失败，错误: %v", err)
		return fmt.Errorf("保存解读报告失败: %w", err)
	}

	log.Infof("并发解读报告分数计算完成，答卷ID: %d", data.AnswerSheetID)
	return nil
}

// loadAnswerSheet 加载答卷
func (h *GenerateInterpretReportHandlerConcurrent) loadAnswerSheet(ctx context.Context, answerSheetID uint64) (*answersheetpb.AnswerSheet, error) {
	answerSheet, err := h.answersheetClient.GetAnswerSheet(ctx, answerSheetID)
	if err != nil {
		return nil, fmt.Errorf("获取答卷失败，ID: %d, 错误: %v", answerSheetID, err)
	}
	return answerSheet, nil
}

// loadMedicalScale 加载医学量表
func (h *GenerateInterpretReportHandlerConcurrent) loadMedicalScale(ctx context.Context, questionnaireCode string) (*medicalscalepb.MedicalScale, error) {
	medicalScale, err := h.medicalScaleClient.GetMedicalScaleByQuestionnaireCode(ctx, questionnaireCode)
	if err != nil {
		return nil, fmt.Errorf("获取医学量表失败，代码: %s, 错误: %v", questionnaireCode, err)
	}
	return medicalScale, nil
}

// buildInterpretItems 构建解读项目
func (h *GenerateInterpretReportHandlerConcurrent) buildInterpretItems(medicalScale *medicalscalepb.MedicalScale) []*interpretreportpb.InterpretItem {
	var interpretItems []*interpretreportpb.InterpretItem

	for _, factor := range medicalScale.Factors {
		interpretItem := &interpretreportpb.InterpretItem{
			FactorCode: factor.Code,
			Title:      factor.Title,
			Score:      0,
			Content:    "",
		}
		interpretItems = append(interpretItems, interpretItem)
	}

	return interpretItems
}

// calculateInterpretReportScoreConcurrent 并发计算解读报告分数
func (h *GenerateInterpretReportHandlerConcurrent) calculateInterpretReportScoreConcurrent(ctx context.Context, interpretReport *interpretreportpb.InterpretReport, answerSheet *answersheetpb.AnswerSheet, medicalScale *medicalscalepb.MedicalScale) error {
	log.Infof("开始并发计算因子分，因子数量: %d, 最大并发数: %d", len(interpretReport.InterpretItems), h.maxConcurrency)

	// 创建答案映射
	answerMap := make(map[string]*answersheetpb.Answer)
	for _, answer := range answerSheet.Answers {
		answerMap[answer.QuestionCode] = answer
	}

	// 创建因子映射
	factorMap := make(map[string]*medicalscalepb.Factor)
	for _, factor := range medicalScale.Factors {
		factorMap[factor.Code] = factor
	}

	// 并发计算一级因子分数
	primaryFactorScores, err := h.calculatePrimaryFactorsConcurrent(ctx, interpretReport.InterpretItems, factorMap, answerMap)
	if err != nil {
		return fmt.Errorf("计算一级因子分数失败: %w", err)
	}

	// 并发计算多级因子分数
	err = h.calculateMultilevelFactorsConcurrent(ctx, interpretReport.InterpretItems, factorMap, primaryFactorScores)
	if err != nil {
		return fmt.Errorf("计算多级因子分数失败: %w", err)
	}

	log.Infof("因子分计算完成")
	return nil
}

// calculatePrimaryFactorsConcurrent 并发计算一级因子分数
func (h *GenerateInterpretReportHandlerConcurrent) calculatePrimaryFactorsConcurrent(ctx context.Context, interpretItems []*interpretreportpb.InterpretItem, factorMap map[string]*medicalscalepb.Factor, answerMap map[string]*answersheetpb.Answer) (map[string]float64, error) {
	primaryFactorScores := make(map[string]float64)
	var mutex sync.Mutex

	// 创建工作任务
	taskChan := make(chan *interpretreportpb.InterpretItem, len(interpretItems))
	resultChan := make(chan struct {
		item  *interpretreportpb.InterpretItem
		score float64
		err   error
	}, len(interpretItems))

	// 启动工作协程
	for i := 0; i < h.maxConcurrency; i++ {
		go func(workerID int) {
			for item := range taskChan {
				factor := factorMap[item.FactorCode]
				if factor == nil {
					resultChan <- struct {
						item  *interpretreportpb.InterpretItem
						score float64
						err   error
					}{item, 0, fmt.Errorf("未找到因子 %s", item.FactorCode)}
					continue
				}

				if factor.FactorType == "primary" || factor.FactorType == "first_grade" {
					score, err := h.calculatePrimaryFactorScore(ctx, factor, answerMap)
					resultChan <- struct {
						item  *interpretreportpb.InterpretItem
						score float64
						err   error
					}{item, score, err}
				} else {
					resultChan <- struct {
						item  *interpretreportpb.InterpretItem
						score float64
						err   error
					}{item, 0, nil} // 非一级因子，跳过
				}
			}
		}(i)
	}

	// 发送任务
	primaryTaskCount := 0
	for _, item := range interpretItems {
		factor := factorMap[item.FactorCode]
		if factor != nil && (factor.FactorType == "primary" || factor.FactorType == "first_grade") {
			taskChan <- item
			primaryTaskCount++
		}
	}
	close(taskChan)

	// 收集结果
	for i := 0; i < primaryTaskCount; i++ {
		result := <-resultChan
		if result.err != nil {
			log.Errorf("计算一级因子分数失败，因子: %s, 错误: %v", result.item.FactorCode, result.err)
			continue
		}

		mutex.Lock()
		primaryFactorScores[result.item.FactorCode] = result.score
		result.item.Score = result.score
		mutex.Unlock()

		log.Infof("一级因子 %s 分数: %f", result.item.FactorCode, result.score)
	}

	return primaryFactorScores, nil
}

// calculateMultilevelFactorsConcurrent 并发计算多级因子分数
func (h *GenerateInterpretReportHandlerConcurrent) calculateMultilevelFactorsConcurrent(ctx context.Context, interpretItems []*interpretreportpb.InterpretItem, factorMap map[string]*medicalscalepb.Factor, primaryFactorScores map[string]float64) error {
	// 创建工作任务
	taskChan := make(chan *interpretreportpb.InterpretItem, len(interpretItems))
	resultChan := make(chan struct {
		item  *interpretreportpb.InterpretItem
		score float64
		err   error
	}, len(interpretItems))

	// 启动工作协程
	for i := 0; i < h.maxConcurrency; i++ {
		go func(workerID int) {
			for item := range taskChan {
				factor := factorMap[item.FactorCode]
				if factor == nil {
					resultChan <- struct {
						item  *interpretreportpb.InterpretItem
						score float64
						err   error
					}{item, 0, fmt.Errorf("未找到因子 %s", item.FactorCode)}
					continue
				}

				if factor.FactorType == "multilevel" || factor.FactorType == "second_grade" {
					score, err := h.calculateMultilevelFactorScore(ctx, factor, primaryFactorScores)
					resultChan <- struct {
						item  *interpretreportpb.InterpretItem
						score float64
						err   error
					}{item, score, err}
				} else {
					resultChan <- struct {
						item  *interpretreportpb.InterpretItem
						score float64
						err   error
					}{item, 0, nil} // 非多级因子，跳过
				}
			}
		}(i)
	}

	// 发送任务
	multilevelTaskCount := 0
	for _, item := range interpretItems {
		factor := factorMap[item.FactorCode]
		if factor != nil && (factor.FactorType == "multilevel" || factor.FactorType == "second_grade") {
			taskChan <- item
			multilevelTaskCount++
		}
	}
	close(taskChan)

	// 收集结果
	for i := 0; i < multilevelTaskCount; i++ {
		result := <-resultChan
		if result.err != nil {
			log.Errorf("计算多级因子分数失败，因子: %s, 错误: %v", result.item.FactorCode, result.err)
			continue
		}

		result.item.Score = result.score
		log.Infof("多级因子 %s 分数: %f", result.item.FactorCode, result.score)
	}

	return nil
}

// calculatePrimaryFactorScore 计算一级因子分数
func (h *GenerateInterpretReportHandlerConcurrent) calculatePrimaryFactorScore(ctx context.Context, factor *medicalscalepb.Factor, answerMap map[string]*answersheetpb.Answer) (float64, error) {
	if factor.CalculationRule == nil {
		return 0, fmt.Errorf("因子 %s 没有计算规则", factor.Code)
	}

	formulaType := factor.CalculationRule.FormulaType
	if formulaType == "" {
		return 0, fmt.Errorf("因子 %s 的计算公式类型为空", factor.Code)
	}

	var operands []float64
	for _, sourceCode := range factor.CalculationRule.SourceCodes {
		if answer, exists := answerMap[sourceCode]; exists {
			operands = append(operands, float64(answer.Score))
			log.Debugf("因子 %s 问题 %s 得分: %d", factor.Code, sourceCode, answer.Score)
		} else {
			log.Warnf("未找到问题的答案，因子: %s, 问题代码: %s", factor.Code, sourceCode)
		}
	}

	if len(operands) == 0 {
		log.Warnf("因子 %s 没有找到任何有效的操作数", factor.Code)
		return 0, nil
	}

	// 创建计算规则
	rule := h.createCalculationRule(formulaType)

	// 执行计算
	result, err := h.calculationEngine.Calculate(ctx, operands, rule)
	if err != nil {
		return 0, fmt.Errorf("计算因子分数失败，因子: %s, 错误: %v", factor.Code, err)
	}

	return result.Value, nil
}

// calculateMultilevelFactorScore 计算多级因子分数
func (h *GenerateInterpretReportHandlerConcurrent) calculateMultilevelFactorScore(ctx context.Context, factor *medicalscalepb.Factor, primaryFactorScores map[string]float64) (float64, error) {
	if factor.CalculationRule == nil {
		return 0, fmt.Errorf("因子 %s 没有计算规则", factor.Code)
	}

	formulaType := factor.CalculationRule.FormulaType
	if formulaType == "" {
		return 0, fmt.Errorf("因子 %s 的计算公式类型为空", factor.Code)
	}

	var operands []float64
	for _, sourceCode := range factor.CalculationRule.SourceCodes {
		if score, exists := primaryFactorScores[sourceCode]; exists {
			operands = append(operands, score)
			log.Debugf("多级因子 %s 一级因子 %s 得分: %f", factor.Code, sourceCode, score)
		} else {
			log.Warnf("未找到一级因子得分，多级因子: %s, 一级因子代码: %s", factor.Code, sourceCode)
		}
	}

	if len(operands) == 0 {
		log.Warnf("多级因子 %s 没有找到任何有效的操作数", factor.Code)
		return 0, nil
	}

	// 创建计算规则
	rule := h.createCalculationRule(formulaType)

	// 执行计算
	result, err := h.calculationEngine.Calculate(ctx, operands, rule)
	if err != nil {
		return 0, fmt.Errorf("计算多级因子分数失败，因子: %s, 错误: %v", factor.Code, err)
	}

	return result.Value, nil
}

// createCalculationRule 创建计算规则
func (h *GenerateInterpretReportHandlerConcurrent) createCalculationRule(formulaType string) *rules.CalculationRule {
	strategyName := h.mapFormulaTypeToStrategy(formulaType)
	return rules.NewCalculationRule(strategyName)
}

// mapFormulaTypeToStrategy 映射公式类型到策略名称
func (h *GenerateInterpretReportHandlerConcurrent) mapFormulaTypeToStrategy(formulaType string) string {
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
		return "option"
	}
}

// saveInterpretReport 保存解读报告
func (h *GenerateInterpretReportHandlerConcurrent) saveInterpretReport(ctx context.Context, interpretReport *interpretreportpb.InterpretReport) error {
	_, err := h.interpretReportClient.SaveInterpretReport(
		ctx,
		interpretReport.AnswerSheetId,
		interpretReport.MedicalScaleCode,
		interpretReport.Title,
		interpretReport.Description,
		interpretReport.InterpretItems,
	)
	return err
}
