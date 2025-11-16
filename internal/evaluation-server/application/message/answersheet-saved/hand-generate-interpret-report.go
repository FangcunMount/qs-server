package answersheet_saved

import (
	"context"
	"fmt"

	"github.com/FangcunMount/component-base/pkg/log"
	answersheetpb "github.com/FangcunMount/qs-server/internal/apiserver/interface/grpc/proto/answersheet"
	interpretreportpb "github.com/FangcunMount/qs-server/internal/apiserver/interface/grpc/proto/interpret-report"
	medicalscalepb "github.com/FangcunMount/qs-server/internal/apiserver/interface/grpc/proto/medical-scale"
	calculationapp "github.com/FangcunMount/qs-server/internal/evaluation-server/application/calculation"
	"github.com/FangcunMount/qs-server/internal/evaluation-server/domain/interpretion"
	grpcclient "github.com/FangcunMount/qs-server/internal/evaluation-server/infrastructure/grpc"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/pubsub"
)

// GenerateInterpretReportHandlerConcurrent 并发版本的解读报告生成处理器
type GenerateInterpretReportHandlerConcurrent struct {
	answersheetClient     *grpcclient.AnswerSheetClient
	medicalScaleClient    *grpcclient.MedicalScaleClient
	interpretReportClient *grpcclient.InterpretReportClient
	calculationPort       calculationapp.CalculationPort
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
		calculationPort:       calculationapp.GetConcurrentCalculationPort(maxConcurrency),
	}
}

// NewGenerateInterpretReportHandlerConcurrentWithAdapter 创建并发版本的解读报告生成处理器（自定义适配器）
func NewGenerateInterpretReportHandlerConcurrentWithAdapter(
	answersheetClient *grpcclient.AnswerSheetClient,
	medicalScaleClient *grpcclient.MedicalScaleClient,
	interpretReportClient *grpcclient.InterpretReportClient,
	calculationPort calculationapp.CalculationPort,
) *GenerateInterpretReportHandlerConcurrent {
	return &GenerateInterpretReportHandlerConcurrent{
		answersheetClient:     answersheetClient,
		medicalScaleClient:    medicalScaleClient,
		interpretReportClient: interpretReportClient,
		calculationPort:       calculationPort,
	}
}

// Handle 并发处理解读报告生成
func (h *GenerateInterpretReportHandlerConcurrent) Handle(ctx context.Context, data pubsub.AnswersheetSavedData) error {
	log.Infof("开始并发计算解读报告分数，答卷ID: %d, 问卷代码: %s", data.AnswerSheetID, data.QuestionnaireCode)

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
	answerSheet, err := h.answersheetClient.GetAnswerSheet(ctx, meta.ID(answerSheetID))
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

// calculateInterpretReportScoreConcurrent 并发计算解读报告分数（业务逻辑层）
func (h *GenerateInterpretReportHandlerConcurrent) calculateInterpretReportScoreConcurrent(ctx context.Context, interpretReport *interpretreportpb.InterpretReport, answerSheet *answersheetpb.AnswerSheet, medicalScale *medicalscalepb.MedicalScale) error {
	log.Infof("开始并发计算因子分，因子数量: %d", len(interpretReport.InterpretItems))

	// 创建答案映射
	answerMap := make(map[string]*answersheetpb.Answer)
	for _, answer := range answerSheet.Answers {
		answerMap[answer.QuestionCode] = answer
	}

	// 分离一级和多级因子
	primaryFactors := []*medicalscalepb.Factor{}
	multilevelFactors := []*medicalscalepb.Factor{}

	for _, factor := range medicalScale.Factors {
		if factor.FactorType == "primary" || factor.FactorType == "first_grade" {
			primaryFactors = append(primaryFactors, factor)
		} else if factor.FactorType == "multilevel" || factor.FactorType == "second_grade" {
			multilevelFactors = append(multilevelFactors, factor)
		}
	}

	// 第一轮：并发计算一级因子分数
	primaryFactorScores, err := h.calculatePrimaryFactorsConcurrent(ctx, primaryFactors, answerMap)
	if err != nil {
		return fmt.Errorf("并发计算一级因子分数失败: %w", err)
	}

	// 应用一级因子分数到解读项
	h.applyFactorScoresToInterpretItems(interpretReport.InterpretItems, primaryFactorScores, "并发primary")

	// 第二轮：并发计算多级因子分数
	multilevelFactorScores, err := h.calculateMultilevelFactorsConcurrent(ctx, multilevelFactors, primaryFactorScores)
	if err != nil {
		return fmt.Errorf("并发计算多级因子分数失败: %w", err)
	}

	// 应用多级因子分数到解读项
	h.applyFactorScoresToInterpretItems(interpretReport.InterpretItems, multilevelFactorScores, "并发multilevel")

	log.Infof("并发因子分计算完成")
	return nil
}

// calculatePrimaryFactorsConcurrent 并发计算一级因子分数
func (h *GenerateInterpretReportHandlerConcurrent) calculatePrimaryFactorsConcurrent(ctx context.Context, factors []*medicalscalepb.Factor, answerMap map[string]*answersheetpb.Answer) (map[string]float64, error) {
	log.Infof("开始并发计算一级因子，因子数量: %d", len(factors))

	// 转换为计算请求
	requests, err := h.convertFactorBatchCalculation(factors, answerMap, nil)
	if err != nil {
		return nil, fmt.Errorf("转换计算请求失败: %w", err)
	}

	if len(requests) == 0 {
		return make(map[string]float64), nil
	}

	// 使用计算端口进行并发批量计算
	results, err := h.calculationPort.CalculateBatch(ctx, requests)
	if err != nil {
		return nil, fmt.Errorf("并发计算失败: %w", err)
	}

	return h.extractFactorScoresFromResults(results), nil
}

// calculateMultilevelFactorsConcurrent 并发计算多级因子分数
func (h *GenerateInterpretReportHandlerConcurrent) calculateMultilevelFactorsConcurrent(ctx context.Context, factors []*medicalscalepb.Factor, primaryFactorScores map[string]float64) (map[string]float64, error) {
	log.Infof("开始并发计算多级因子，因子数量: %d", len(factors))

	// 转换为计算请求
	requests, err := h.convertFactorBatchCalculation(factors, nil, primaryFactorScores)
	if err != nil {
		return nil, fmt.Errorf("转换计算请求失败: %w", err)
	}

	if len(requests) == 0 {
		return make(map[string]float64), nil
	}

	// 使用计算端口进行并发批量计算
	results, err := h.calculationPort.CalculateBatch(ctx, requests)
	if err != nil {
		return nil, fmt.Errorf("并发计算失败: %w", err)
	}

	return h.extractFactorScoresFromResults(results), nil
}

// extractFactorScoresFromResults 从结果中提取因子分数
func (h *GenerateInterpretReportHandlerConcurrent) extractFactorScoresFromResults(results []*calculationapp.CalculationResult) map[string]float64 {
	factorScores := make(map[string]float64)

	for _, result := range results {
		if result.Error != "" {
			log.Errorf("计算失败，任务: %s, 错误: %s", result.Name, result.Error)
			continue
		}

		if factorCode := extractFactorCodeFromResultIDConcurrent(result.ID); factorCode != "" {
			factorScores[factorCode] = result.Value
			log.Debugf("因子 %s 分数: %f", factorCode, result.Value)
		}
	}

	return factorScores
}

// extractFactorCodeFromResultIDConcurrent 从结果ID提取因子代码（并发版本）
func extractFactorCodeFromResultIDConcurrent(resultID string) string {
	const prefix = "factor_"
	if len(resultID) > len(prefix) && resultID[:len(prefix)] == prefix {
		return resultID[len(prefix):]
	}
	return ""
}

// applyFactorScoresToInterpretItems 应用因子分数到解读项
func (h *GenerateInterpretReportHandlerConcurrent) applyFactorScoresToInterpretItems(interpretItems []*interpretreportpb.InterpretItem, factorScores map[string]float64, factorType string) {
	// 创建解读项映射
	itemMap := make(map[string]*interpretreportpb.InterpretItem)
	for _, item := range interpretItems {
		itemMap[item.FactorCode] = item
	}

	successCount := 0
	for factorCode, score := range factorScores {
		if item, exists := itemMap[factorCode]; exists {
			item.Score = score
			successCount++
			log.Infof("%s因子 %s 分数: %f", factorType, factorCode, score)
		}
	}

	log.Infof("%s因子计算完成，成功 %d 个", factorType, successCount)
}

// saveInterpretReport 保存解读报告
func (h *GenerateInterpretReportHandlerConcurrent) saveInterpretReport(ctx context.Context, interpretReport *interpretreportpb.InterpretReport) error {
	_, err := h.interpretReportClient.SaveInterpretReport(
		ctx,
		meta.ID(interpretReport.AnswerSheetId),
		interpretReport.MedicalScaleCode,
		interpretReport.Title,
		interpretReport.Description,
		interpretReport.InterpretItems,
	)
	return err
}

// SetCalculationPort 设置计算端口（运行时切换适配器）
func (h *GenerateInterpretReportHandlerConcurrent) SetCalculationPort(port calculationapp.CalculationPort) {
	h.calculationPort = port
}

// convertFactorBatchCalculation 批量转换因子计算请求（私有方法）
func (h *GenerateInterpretReportHandlerConcurrent) convertFactorBatchCalculation(factors []*medicalscalepb.Factor, answerMap map[string]*answersheetpb.Answer, factorScores map[string]float64) ([]*calculationapp.CalculationRequest, error) {
	if len(factors) == 0 {
		return []*calculationapp.CalculationRequest{}, nil
	}

	var requests []*calculationapp.CalculationRequest

	for _, factor := range factors {
		if factor.CalculationRule == nil || factor.CalculationRule.FormulaType == "" {
			log.Debugf("因子 %s 无需计算", factor.Code)
			continue
		}

		// 根据因子类型选择操作数源
		operandData := make(map[string]float64)

		if factor.FactorType == "primary" || factor.FactorType == "first_grade" {
			// 一级因子：使用答案得分
			for _, sourceCode := range factor.CalculationRule.SourceCodes {
				if answer, exists := answerMap[sourceCode]; exists {
					operandData[sourceCode] = float64(answer.Score)
				}
			}
		} else if factor.FactorType == "multilevel" || factor.FactorType == "second_grade" {
			// 多级因子：使用其他因子得分
			for _, sourceCode := range factor.CalculationRule.SourceCodes {
				if score, exists := factorScores[sourceCode]; exists {
					operandData[sourceCode] = score
				}
			}
		}

		request, err := h.convertFactorCalculation(factor, operandData)
		if err != nil {
			log.Errorf("转换因子计算请求失败，因子: %s, 错误: %v", factor.Code, err)
			continue
		}

		requests = append(requests, request)
	}

	log.Infof("批量转换因子计算请求完成，共生成 %d 个计算任务", len(requests))
	return requests, nil
}

// convertFactorCalculation 转换因子计算请求（私有方法）
func (h *GenerateInterpretReportHandlerConcurrent) convertFactorCalculation(factor *medicalscalepb.Factor, operandData map[string]float64) (*calculationapp.CalculationRequest, error) {
	if factor == nil {
		return nil, fmt.Errorf("因子不能为空")
	}

	if factor.CalculationRule == nil || factor.CalculationRule.FormulaType == "" {
		return nil, fmt.Errorf("因子 %s 没有有效的计算规则", factor.Code)
	}

	// 根据源代码列表提取操作数
	var operands []float64
	for _, sourceCode := range factor.CalculationRule.SourceCodes {
		if value, exists := operandData[sourceCode]; exists {
			operands = append(operands, value)
		} else {
			log.Warnf("未找到源代码对应的操作数: %s", sourceCode)
		}
	}

	if len(operands) == 0 {
		return nil, fmt.Errorf("因子 %s 没有有效的操作数", factor.Code)
	}

	return &calculationapp.CalculationRequest{
		ID:          fmt.Sprintf("factor_%s", factor.Code),
		Name:        fmt.Sprintf("因子 %s 计算", factor.Title),
		FormulaType: factor.CalculationRule.FormulaType,
		Operands:    operands,
		Parameters: map[string]interface{}{
			"factor_code":  factor.Code,
			"factor_type":  factor.FactorType,
			"source_codes": factor.CalculationRule.SourceCodes,
			"total_score":  factor.IsTotalScore,
		},
		Precision:    2,
		RoundingMode: "round",
	}, nil
}
