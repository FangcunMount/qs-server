package answersheet_saved

import (
	"context"
	"fmt"
	"sync"
	"time"

	answersheetpb "github.com/yshujie/questionnaire-scale/internal/apiserver/interface/grpc/proto/answersheet"
	interpretreportpb "github.com/yshujie/questionnaire-scale/internal/apiserver/interface/grpc/proto/interpret-report"
	medicalscalepb "github.com/yshujie/questionnaire-scale/internal/apiserver/interface/grpc/proto/medical-scale"
	"github.com/yshujie/questionnaire-scale/internal/evaluation-server/domain/calculation"
	"github.com/yshujie/questionnaire-scale/internal/evaluation-server/domain/interpretion"
	grpcclient "github.com/yshujie/questionnaire-scale/internal/evaluation-server/infrastructure/grpc"
	"github.com/yshujie/questionnaire-scale/internal/pkg/pubsub"
	"github.com/yshujie/questionnaire-scale/pkg/log"
)

// GenerateInterpretReportHandlerConcurrent 并发优化版本的解读报告生成处理器
type GenerateInterpretReportHandlerConcurrent struct {
	answersheetClient     *grpcclient.AnswerSheetClient
	medicalScaleClient    *grpcclient.MedicalScaleClient
	interpretReportClient *grpcclient.InterpretReportClient
	maxConcurrency        int // 最大并发数
}

// NewGenerateInterpretReportHandlerConcurrent 创建并发优化版本的解读报告生成处理器
func NewGenerateInterpretReportHandlerConcurrent(
	answersheetClient *grpcclient.AnswerSheetClient,
	medicalScaleClient *grpcclient.MedicalScaleClient,
	interpretReportClient *grpcclient.InterpretReportClient,
	maxConcurrency int,
) *GenerateInterpretReportHandlerConcurrent {
	if maxConcurrency <= 0 {
		maxConcurrency = 10 // 默认并发数
	}
	return &GenerateInterpretReportHandlerConcurrent{
		answersheetClient:     answersheetClient,
		medicalScaleClient:    medicalScaleClient,
		interpretReportClient: interpretReportClient,
		maxConcurrency:        maxConcurrency,
	}
}

// Handle 并发计算解读报告中的因子分，并保存解读报告
func (h *GenerateInterpretReportHandlerConcurrent) Handle(ctx context.Context, data pubsub.AnswersheetSavedData) error {
	startTime := time.Now()
	log.Infof("开始并发计算解读报告分数，答卷ID: %d, 问卷代码: %s", data.AnswerSheetID, data.QuestionnaireCode)

	// 1. 并发加载数据
	loadStartTime := time.Now()
	answerSheet, medicalScale, err := h.loadDataConcurrently(ctx, data.AnswerSheetID, data.QuestionnaireCode)
	if err != nil {
		return fmt.Errorf("并发加载数据失败: %w", err)
	}
	loadDuration := time.Since(loadStartTime)

	// 2. 创建解读报告
	interpretReport := &interpretreportpb.InterpretReport{
		Id:               data.AnswerSheetID,
		AnswerSheetId:    data.AnswerSheetID,
		MedicalScaleCode: data.QuestionnaireCode,
		Title:            medicalScale.Title,
		Description:      medicalScale.Description,
		InterpretItems:   h.buildInterpretItems(medicalScale),
	}

	// 3. 并发计算因子分数
	calcStartTime := time.Now()
	if err := h.calculateInterpretReportScoreConcurrently(interpretReport, answerSheet, medicalScale); err != nil {
		return fmt.Errorf("并发计算因子分数失败: %w", err)
	}
	calcDuration := time.Since(calcStartTime)

	// 4. 并发生成解读内容
	contentStartTime := time.Now()
	if err := h.generateInterpretContentConcurrently(interpretReport, medicalScale); err != nil {
		return fmt.Errorf("并发生成解读内容失败: %w", err)
	}
	contentDuration := time.Since(contentStartTime)

	// 5. 保存解读报告
	saveStartTime := time.Now()
	if err := h.saveInterpretReport(ctx, interpretReport); err != nil {
		return fmt.Errorf("保存解读报告失败: %w", err)
	}
	saveDuration := time.Since(saveStartTime)

	totalDuration := time.Since(startTime)
	log.Infof("解读报告并发计算完成，答卷ID: %d, 总耗时: %v (加载: %v, 计算: %v, 内容生成: %v, 保存: %v)",
		data.AnswerSheetID, totalDuration, loadDuration, calcDuration, contentDuration, saveDuration)

	return nil
}

// loadDataConcurrently 并发加载答卷和医学量表数据
func (h *GenerateInterpretReportHandlerConcurrent) loadDataConcurrently(ctx context.Context, answerSheetID uint64, medicalScaleCode string) (*answersheetpb.AnswerSheet, *medicalscalepb.MedicalScale, error) {
	type loadResult struct {
		answerSheet  *answersheetpb.AnswerSheet
		medicalScale *medicalscalepb.MedicalScale
		err          error
	}

	resultChan := make(chan loadResult, 2)

	// 并发加载答卷
	go func() {
		answerSheet, err := h.loadAnswerSheet(ctx, answerSheetID)
		resultChan <- loadResult{answerSheet: answerSheet, err: err}
	}()

	// 并发加载医学量表
	go func() {
		medicalScale, err := h.loadMedicalScale(ctx, medicalScaleCode)
		resultChan <- loadResult{medicalScale: medicalScale, err: err}
	}()

	// 收集结果
	var answerSheet *answersheetpb.AnswerSheet
	var medicalScale *medicalscalepb.MedicalScale

	for i := 0; i < 2; i++ {
		result := <-resultChan
		if result.err != nil {
			return nil, nil, result.err
		}
		if result.answerSheet != nil {
			answerSheet = result.answerSheet
		}
		if result.medicalScale != nil {
			medicalScale = result.medicalScale
		}
	}

	return answerSheet, medicalScale, nil
}

// calculateInterpretReportScoreConcurrently 并发计算解读报告中的因子分
func (h *GenerateInterpretReportHandlerConcurrent) calculateInterpretReportScoreConcurrently(interpretReport *interpretreportpb.InterpretReport, answerSheet *answersheetpb.AnswerSheet, medicalScale *medicalscalepb.MedicalScale) error {
	log.Infof("开始并发计算因子分，因子数量: %d", len(interpretReport.InterpretItems))

	// 创建答案映射和因子映射
	answerMap := make(map[string]*answersheetpb.Answer)
	for _, answer := range answerSheet.Answers {
		answerMap[answer.QuestionCode] = answer
	}

	factorMap := make(map[string]*medicalscalepb.Factor)
	for _, factor := range medicalScale.Factors {
		factorMap[factor.Code] = factor
	}

	// 分离一级因子和多级因子
	var primaryFactors []*interpretreportpb.InterpretItem
	var multilevelFactors []*interpretreportpb.InterpretItem

	for _, interpretItem := range interpretReport.InterpretItems {
		factor := factorMap[interpretItem.FactorCode]
		if factor == nil {
			log.Warnf("未找到因子，代码: %s", interpretItem.FactorCode)
			continue
		}

		if factor.FactorType == "primary" || factor.FactorType == "first_grade" {
			primaryFactors = append(primaryFactors, interpretItem)
		} else if factor.FactorType == "multilevel" || factor.FactorType == "second_grade" {
			multilevelFactors = append(multilevelFactors, interpretItem)
		}
	}

	// 第一轮：并发计算一级因子分数
	primaryFactorScores := h.calculatePrimaryFactorsConcurrently(primaryFactors, factorMap, answerMap)

	// 第二轮：并发计算多级因子分数
	h.calculateMultilevelFactorsConcurrently(multilevelFactors, factorMap, primaryFactorScores)

	return nil
}

// calculatePrimaryFactorsConcurrently 并发计算一级因子分数
func (h *GenerateInterpretReportHandlerConcurrent) calculatePrimaryFactorsConcurrently(
	primaryFactors []*interpretreportpb.InterpretItem,
	factorMap map[string]*medicalscalepb.Factor,
	answerMap map[string]*answersheetpb.Answer,
) map[string]float64 {
	if len(primaryFactors) == 0 {
		return make(map[string]float64)
	}

	// 使用 worker pool 模式并发计算
	type factorResult struct {
		factorCode string
		score      float64
		err        error
	}

	taskChan := make(chan *interpretreportpb.InterpretItem, len(primaryFactors))
	resultChan := make(chan factorResult, len(primaryFactors))

	// 启动 worker goroutines
	var wg sync.WaitGroup
	for i := 0; i < h.maxConcurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for interpretItem := range taskChan {
				factor := factorMap[interpretItem.FactorCode]
				if factor == nil {
					continue
				}

				score, err := h.calculatePrimaryFactorScore(factor, answerMap)
				resultChan <- factorResult{
					factorCode: interpretItem.FactorCode,
					score:      score,
					err:        err,
				}
			}
		}(i)
	}

	// 发送任务
	for _, interpretItem := range primaryFactors {
		taskChan <- interpretItem
	}
	close(taskChan)

	// 等待所有 worker 完成
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// 收集结果
	primaryFactorScores := make(map[string]float64)
	for result := range resultChan {
		if result.err != nil {
			log.Errorf("计算一级因子分数失败，因子: %s, 错误: %v", result.factorCode, result.err)
			continue
		}
		primaryFactorScores[result.factorCode] = result.score

		// 更新解读项分数
		for _, interpretItem := range primaryFactors {
			if interpretItem.FactorCode == result.factorCode {
				interpretItem.Score = result.score
				log.Infof("一级因子 %s 分数: %f", result.factorCode, result.score)
				break
			}
		}
	}

	return primaryFactorScores
}

// calculateMultilevelFactorsConcurrently 并发计算多级因子分数
func (h *GenerateInterpretReportHandlerConcurrent) calculateMultilevelFactorsConcurrently(
	multilevelFactors []*interpretreportpb.InterpretItem,
	factorMap map[string]*medicalscalepb.Factor,
	primaryFactorScores map[string]float64,
) {
	if len(multilevelFactors) == 0 {
		return
	}

	// 使用 worker pool 模式并发计算
	type factorResult struct {
		factorCode string
		score      float64
		err        error
	}

	taskChan := make(chan *interpretreportpb.InterpretItem, len(multilevelFactors))
	resultChan := make(chan factorResult, len(multilevelFactors))

	// 启动 worker goroutines
	var wg sync.WaitGroup
	for i := 0; i < h.maxConcurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for interpretItem := range taskChan {
				factor := factorMap[interpretItem.FactorCode]
				if factor == nil {
					continue
				}

				score, err := h.calculateMultilevelFactorScore(factor, primaryFactorScores)
				resultChan <- factorResult{
					factorCode: interpretItem.FactorCode,
					score:      score,
					err:        err,
				}
			}
		}(i)
	}

	// 发送任务
	for _, interpretItem := range multilevelFactors {
		taskChan <- interpretItem
	}
	close(taskChan)

	// 等待所有 worker 完成
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// 收集结果
	for result := range resultChan {
		if result.err != nil {
			log.Errorf("计算多级因子分数失败，因子: %s, 错误: %v", result.factorCode, result.err)
			continue
		}

		// 更新解读项分数
		for _, interpretItem := range multilevelFactors {
			if interpretItem.FactorCode == result.factorCode {
				interpretItem.Score = result.score
				log.Infof("多级因子 %s 分数: %f", result.factorCode, result.score)
				break
			}
		}
	}
}

// generateInterpretContentConcurrently 并发生成解读内容
func (h *GenerateInterpretReportHandlerConcurrent) generateInterpretContentConcurrently(interpretReport *interpretreportpb.InterpretReport, medicalScale *medicalscalepb.MedicalScale) error {
	// 创建内容生成器
	contentGenerator := interpretion.NewInterpretReportContentGenerator()

	// 生成解读内容（这个操作本身可能已经是并发的，取决于具体实现）
	return contentGenerator.GenerateInterpretContent(interpretReport, medicalScale)
}

// 以下方法从原版本复制，保持不变
func (h *GenerateInterpretReportHandlerConcurrent) loadAnswerSheet(ctx context.Context, answerSheetID uint64) (*answersheetpb.AnswerSheet, error) {
	answerSheet, err := h.answersheetClient.GetAnswerSheet(ctx, answerSheetID)
	if err != nil {
		return nil, err
	}
	if answerSheet == nil {
		return nil, fmt.Errorf("答卷不存在，ID: %d", answerSheetID)
	}
	return answerSheet, nil
}

func (h *GenerateInterpretReportHandlerConcurrent) loadMedicalScale(ctx context.Context, medicalScaleCode string) (*medicalscalepb.MedicalScale, error) {
	medicalScale, err := h.medicalScaleClient.GetMedicalScaleByQuestionnaireCode(ctx, medicalScaleCode)
	if err != nil {
		return nil, err
	}
	if medicalScale == nil {
		return nil, fmt.Errorf("医学量表不存在，代码: %s", medicalScaleCode)
	}
	return medicalScale, nil
}

func (h *GenerateInterpretReportHandlerConcurrent) buildInterpretItems(medicalScale *medicalscalepb.MedicalScale) []*interpretreportpb.InterpretItem {
	interpretItems := make([]*interpretreportpb.InterpretItem, 0, len(medicalScale.Factors))
	for _, factor := range medicalScale.Factors {
		content := fmt.Sprintf("因子 %s (%s) 的评估结果", factor.Title, factor.Code)
		interpretItems = append(interpretItems, &interpretreportpb.InterpretItem{
			FactorCode: factor.Code,
			Title:      factor.Title,
			Score:      0,
			Content:    content,
		})
	}
	return interpretItems
}

func (h *GenerateInterpretReportHandlerConcurrent) calculatePrimaryFactorScore(factor *medicalscalepb.Factor, answerMap map[string]*answersheetpb.Answer) (float64, error) {
	if factor.CalculationRule == nil {
		return 0, fmt.Errorf("因子 %s 没有计算规则", factor.Code)
	}

	formulaType := factor.CalculationRule.FormulaType
	if formulaType == "" {
		return 0, fmt.Errorf("因子 %s 的计算公式类型为空", factor.Code)
	}

	calculater, err := calculation.GetCalculater(calculation.CalculaterType(formulaType))
	if err != nil {
		return 0, fmt.Errorf("获取计算器失败，公式类型: %s, 错误: %v", formulaType, err)
	}

	var operands []calculation.Operand
	for _, sourceCode := range factor.CalculationRule.SourceCodes {
		if answer, exists := answerMap[sourceCode]; exists {
			operands = append(operands, calculation.Operand(answer.Score))
			log.Debugf("因子 %s 问题 %s 得分: %d", factor.Code, sourceCode, answer.Score)
		} else {
			log.Warnf("未找到问题的答案，因子: %s, 问题代码: %s", factor.Code, sourceCode)
		}
	}

	if len(operands) == 0 {
		log.Warnf("因子 %s 没有找到任何有效的操作数", factor.Code)
		return 0, nil
	}

	score, err := calculater.Calculate(operands)
	if err != nil {
		return 0, fmt.Errorf("计算因子分数失败，因子: %s, 错误: %v", factor.Code, err)
	}

	return score.Value(), nil
}

func (h *GenerateInterpretReportHandlerConcurrent) calculateMultilevelFactorScore(factor *medicalscalepb.Factor, primaryFactorScores map[string]float64) (float64, error) {
	if factor.CalculationRule == nil {
		return 0, fmt.Errorf("因子 %s 没有计算规则", factor.Code)
	}

	formulaType := factor.CalculationRule.FormulaType
	if formulaType == "" {
		return 0, fmt.Errorf("因子 %s 的计算公式类型为空", factor.Code)
	}

	calculater, err := calculation.GetCalculater(calculation.CalculaterType(formulaType))
	if err != nil {
		return 0, fmt.Errorf("获取计算器失败，公式类型: %s, 错误: %v", formulaType, err)
	}

	var operands []calculation.Operand
	for _, sourceCode := range factor.CalculationRule.SourceCodes {
		if score, exists := primaryFactorScores[sourceCode]; exists {
			operands = append(operands, calculation.Operand(score))
			log.Debugf("多级因子 %s 一级因子 %s 得分: %f", factor.Code, sourceCode, score)
		} else {
			log.Warnf("未找到一级因子得分，多级因子: %s, 一级因子代码: %s", factor.Code, sourceCode)
		}
	}

	if len(operands) == 0 {
		log.Warnf("多级因子 %s 没有找到任何有效的操作数", factor.Code)
		return 0, nil
	}

	score, err := calculater.Calculate(operands)
	if err != nil {
		return 0, fmt.Errorf("计算多级因子分数失败，因子: %s, 错误: %v", factor.Code, err)
	}

	return score.Value(), nil
}

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
