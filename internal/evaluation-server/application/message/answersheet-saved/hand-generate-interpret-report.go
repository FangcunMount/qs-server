package answersheet_saved

import (
	"context"
	"fmt"

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

// HandlerGenerateInterpretReport 生成解读报告处理器
type GenerateInterpretReportHandler struct {
	answersheetClient     *grpcclient.AnswerSheetClient
	medicalScaleClient    *grpcclient.MedicalScaleClient
	interpretReportClient *grpcclient.InterpretReportClient
	calculationEngine     *calculation.CalculationEngine
}

// NewGenerateInterpretReportHandler 创建生成解读报告处理器
func NewGenerateInterpretReportHandler(
	answersheetClient *grpcclient.AnswerSheetClient,
	medicalScaleClient *grpcclient.MedicalScaleClient,
	interpretReportClient *grpcclient.InterpretReportClient,
) *GenerateInterpretReportHandler {
	return &GenerateInterpretReportHandler{
		answersheetClient:     answersheetClient,
		medicalScaleClient:    medicalScaleClient,
		interpretReportClient: interpretReportClient,
		calculationEngine:     calculation.GetGlobalCalculationEngine(),
	}
}

// Handle 计算解读报告中的因子分，并保存解读报告
func (h *GenerateInterpretReportHandler) Handle(ctx context.Context, data pubsub.AnswersheetSavedData) error {
	log.Infof("开始计算解读报告分数，答卷ID: %d, 问卷代码: %s", data.AnswerSheetID, data.QuestionnaireCode)

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

	// 计算解读报告中的因子分
	if err := h.calculateInterpretReportScore(ctx, interpretReport, answerSheet, medicalScale); err != nil {
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

	log.Infof("解读报告分数计算完成，答卷ID: %d", data.AnswerSheetID)
	return nil
}

// loadAnswerSheet 加载答卷
func (h *GenerateInterpretReportHandler) loadAnswerSheet(ctx context.Context, answerSheetID uint64) (*answersheetpb.AnswerSheet, error) {
	answerSheet, err := h.answersheetClient.GetAnswerSheet(ctx, answerSheetID)
	if err != nil {
		return nil, fmt.Errorf("获取答卷失败，ID: %d, 错误: %v", answerSheetID, err)
	}
	return answerSheet, nil
}

// loadMedicalScale 加载医学量表
func (h *GenerateInterpretReportHandler) loadMedicalScale(ctx context.Context, questionnaireCode string) (*medicalscalepb.MedicalScale, error) {
	medicalScale, err := h.medicalScaleClient.GetMedicalScaleByQuestionnaireCode(ctx, questionnaireCode)
	if err != nil {
		return nil, fmt.Errorf("获取医学量表失败，代码: %s, 错误: %v", questionnaireCode, err)
	}
	return medicalScale, nil
}

// buildInterpretItems 构建解读项目
func (h *GenerateInterpretReportHandler) buildInterpretItems(medicalScale *medicalscalepb.MedicalScale) []*interpretreportpb.InterpretItem {
	var interpretItems []*interpretreportpb.InterpretItem

	for _, factor := range medicalScale.Factors {
		interpretItem := &interpretreportpb.InterpretItem{
			FactorCode: factor.Code,
			Title:      factor.Title,
			Score:      0,  // 初始分数为0，后续会计算
			Content:    "", // 初始内容为空，后续会生成
		}
		interpretItems = append(interpretItems, interpretItem)
	}

	return interpretItems
}

// calculateInterpretReportScore 计算解读报告分数
func (h *GenerateInterpretReportHandler) calculateInterpretReportScore(ctx context.Context, interpretReport *interpretreportpb.InterpretReport, answerSheet *answersheetpb.AnswerSheet, medicalScale *medicalscalepb.MedicalScale) error {
	log.Infof("开始计算因子分，因子数量: %d", len(interpretReport.InterpretItems))

	// 创建答案映射，便于快速查找
	answerMap := make(map[string]*answersheetpb.Answer)
	for _, answer := range answerSheet.Answers {
		answerMap[answer.QuestionCode] = answer
	}

	// 创建因子映射，便于快速查找
	factorMap := make(map[string]*medicalscalepb.Factor)
	for _, factor := range medicalScale.Factors {
		factorMap[factor.Code] = factor
	}

	// 第一轮：计算一级因子分数
	primaryFactorScores := make(map[string]float64)
	for _, interpretItem := range interpretReport.InterpretItems {
		factor := factorMap[interpretItem.FactorCode]
		if factor == nil {
			log.Warnf("未找到因子，代码: %s", interpretItem.FactorCode)
			continue
		}

		// 判断因子类型
		if factor.FactorType == "primary" || factor.FactorType == "first_grade" {
			// 一级因子：根据计算公式和问题答案计算
			score, err := h.calculatePrimaryFactorScore(ctx, factor, answerMap)
			if err != nil {
				log.Errorf("计算一级因子分数失败，因子: %s, 错误: %v", factor.Code, err)
				continue
			}
			primaryFactorScores[factor.Code] = score
			interpretItem.Score = score
			log.Infof("一级因子 %s 分数: %f", factor.Code, score)
		}
	}

	// 第二轮：计算多级因子分数
	for _, interpretItem := range interpretReport.InterpretItems {
		factor := factorMap[interpretItem.FactorCode]
		if factor == nil {
			continue
		}

		// 判断因子类型
		if factor.FactorType == "multilevel" || factor.FactorType == "second_grade" {
			// 多级因子：根据一级因子分数计算
			score, err := h.calculateMultilevelFactorScore(ctx, factor, primaryFactorScores)
			if err != nil {
				log.Errorf("计算多级因子分数失败，因子: %s, 错误: %v", factor.Code, err)
				continue
			}
			interpretItem.Score = score
			log.Infof("多级因子 %s 分数: %f", factor.Code, score)
		}
	}

	log.Infof("因子分计算完成")
	return nil
}

// calculatePrimaryFactorScore 计算一级因子分数
func (h *GenerateInterpretReportHandler) calculatePrimaryFactorScore(ctx context.Context, factor *medicalscalepb.Factor, answerMap map[string]*answersheetpb.Answer) (float64, error) {
	if factor.CalculationRule == nil {
		return 0, fmt.Errorf("因子 %s 没有计算规则", factor.Code)
	}

	// 获取计算公式类型
	formulaType := factor.CalculationRule.FormulaType
	if formulaType == "" {
		return 0, fmt.Errorf("因子 %s 的计算公式类型为空", factor.Code)
	}

	// 获取计算操作数（根据因子的源代码列表和对应的答案得分）
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
func (h *GenerateInterpretReportHandler) calculateMultilevelFactorScore(ctx context.Context, factor *medicalscalepb.Factor, primaryFactorScores map[string]float64) (float64, error) {
	if factor.CalculationRule == nil {
		return 0, fmt.Errorf("因子 %s 没有计算规则", factor.Code)
	}

	// 获取计算公式类型
	formulaType := factor.CalculationRule.FormulaType
	if formulaType == "" {
		return 0, fmt.Errorf("因子 %s 的计算公式类型为空", factor.Code)
	}

	// 获取计算操作数（根据因子的源代码列表和对应的一级因子得分）
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
func (h *GenerateInterpretReportHandler) createCalculationRule(formulaType string) *rules.CalculationRule {
	// 将旧的公式类型映射到新的策略名称
	strategyName := h.mapFormulaTypeToStrategy(formulaType)
	return rules.NewCalculationRule(strategyName)
}

// mapFormulaTypeToStrategy 映射公式类型到策略名称
func (h *GenerateInterpretReportHandler) mapFormulaTypeToStrategy(formulaType string) string {
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

// saveInterpretReport 保存解读报告
func (h *GenerateInterpretReportHandler) saveInterpretReport(ctx context.Context, interpretReport *interpretreportpb.InterpretReport) error {
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
