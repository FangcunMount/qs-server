package message

import (
	"context"
	"fmt"

	answersheetpb "github.com/yshujie/questionnaire-scale/internal/apiserver/interface/grpc/proto/answersheet"
	interpretreportpb "github.com/yshujie/questionnaire-scale/internal/apiserver/interface/grpc/proto/interpret-report"
	medicalscalepb "github.com/yshujie/questionnaire-scale/internal/apiserver/interface/grpc/proto/medical-scale"
	"github.com/yshujie/questionnaire-scale/internal/evaluation-server/domain/calculation"
	grpcclient "github.com/yshujie/questionnaire-scale/internal/evaluation-server/infrastructure/grpc"
	"github.com/yshujie/questionnaire-scale/internal/pkg/pubsub"
	"github.com/yshujie/questionnaire-scale/pkg/log"
)

// HandlerCalcInterpretReportScore 计算解读报告分数处理器
type HandlerCalcInterpretReportScore struct {
	answersheetClient     *grpcclient.AnswerSheetClient
	medicalScaleClient    *grpcclient.MedicalScaleClient
	interpretReportClient *grpcclient.InterpretReportClient
}

// Handle 计算解读报告中的因子分，并保存解读报告
func (h *HandlerCalcInterpretReportScore) Handle(ctx context.Context, data pubsub.AnswersheetSavedData) error {
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
	if err := h.calculateInterpretReportScore(interpretReport, answerSheet, medicalScale); err != nil {
		log.Errorf("计算解读报告分数失败，错误: %v", err)
		return fmt.Errorf("计算解读报告分数失败: %w", err)
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
func (h *HandlerCalcInterpretReportScore) loadAnswerSheet(ctx context.Context, answerSheetID uint64) (*answersheetpb.AnswerSheet, error) {
	answerSheet, err := h.answersheetClient.GetAnswerSheet(ctx, answerSheetID)
	if err != nil {
		return nil, err
	}
	if answerSheet == nil {
		return nil, fmt.Errorf("答卷不存在，ID: %d", answerSheetID)
	}
	return answerSheet, nil
}

// loadMedicalScale 加载医学量表
func (h *HandlerCalcInterpretReportScore) loadMedicalScale(ctx context.Context, medicalScaleCode string) (*medicalscalepb.MedicalScale, error) {
	medicalScale, err := h.medicalScaleClient.GetMedicalScaleByQuestionnaireCode(ctx, medicalScaleCode)
	if err != nil {
		return nil, err
	}
	if medicalScale == nil {
		return nil, fmt.Errorf("医学量表不存在，代码: %s", medicalScaleCode)
	}
	return medicalScale, nil
}

// buildInterpretItems 构建解读项
func (h *HandlerCalcInterpretReportScore) buildInterpretItems(medicalScale *medicalscalepb.MedicalScale) []*interpretreportpb.InterpretItem {
	interpretItems := make([]*interpretreportpb.InterpretItem, 0, len(medicalScale.Factors))
	for _, factor := range medicalScale.Factors {
		// 为解读项设置默认内容
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

// calculateInterpretReportScore 计算解读报告中的因子分
func (h *HandlerCalcInterpretReportScore) calculateInterpretReportScore(interpretReport *interpretreportpb.InterpretReport, answerSheet *answersheetpb.AnswerSheet, medicalScale *medicalscalepb.MedicalScale) error {
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
			score, err := h.calculatePrimaryFactorScore(factor, answerMap)
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

		if factor.FactorType == "multilevel" || factor.FactorType == "second_grade" {
			// 多级因子：根据计算公式和一级因子得分计算
			score, err := h.calculateMultilevelFactorScore(factor, primaryFactorScores)
			if err != nil {
				log.Errorf("计算多级因子分数失败，因子: %s, 错误: %v", factor.Code, err)
				continue
			}
			interpretItem.Score = score
			log.Infof("多级因子 %s 分数: %f", factor.Code, score)
		}
	}

	return nil
}

// calculatePrimaryFactorScore 计算一级因子分数
func (h *HandlerCalcInterpretReportScore) calculatePrimaryFactorScore(factor *medicalscalepb.Factor, answerMap map[string]*answersheetpb.Answer) (float64, error) {
	if factor.CalculationRule == nil {
		return 0, fmt.Errorf("因子 %s 没有计算规则", factor.Code)
	}

	// 获取计算公式类型
	formulaType := factor.CalculationRule.FormulaType
	if formulaType == "" {
		return 0, fmt.Errorf("因子 %s 的计算公式类型为空", factor.Code)
	}

	// 根据计算规则，创建计算器
	calculater, err := calculation.GetCalculater(calculation.CalculaterType(formulaType))
	if err != nil {
		return 0, fmt.Errorf("获取计算器失败，公式类型: %s, 错误: %v", formulaType, err)
	}

	// 获取计算操作数（根据因子的源代码列表和对应的答案得分）
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

	// 执行计算
	score, err := calculater.Calculate(operands)
	if err != nil {
		return 0, fmt.Errorf("计算因子分数失败，因子: %s, 错误: %v", factor.Code, err)
	}

	return score.Value(), nil
}

// calculateMultilevelFactorScore 计算多级因子分数
func (h *HandlerCalcInterpretReportScore) calculateMultilevelFactorScore(factor *medicalscalepb.Factor, primaryFactorScores map[string]float64) (float64, error) {
	if factor.CalculationRule == nil {
		return 0, fmt.Errorf("因子 %s 没有计算规则", factor.Code)
	}

	// 获取计算公式类型
	formulaType := factor.CalculationRule.FormulaType
	if formulaType == "" {
		return 0, fmt.Errorf("因子 %s 的计算公式类型为空", factor.Code)
	}

	// 根据计算规则，创建计算器
	calculater, err := calculation.GetCalculater(calculation.CalculaterType(formulaType))
	if err != nil {
		return 0, fmt.Errorf("获取计算器失败，公式类型: %s, 错误: %v", formulaType, err)
	}

	// 获取计算操作数（根据因子的源代码列表和对应的一级因子得分）
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

	// 执行计算
	score, err := calculater.Calculate(operands)
	if err != nil {
		return 0, fmt.Errorf("计算多级因子分数失败，因子: %s, 错误: %v", factor.Code, err)
	}

	return score.Value(), nil
}

// saveInterpretReport 保存解读报告
func (h *HandlerCalcInterpretReportScore) saveInterpretReport(ctx context.Context, interpretReport *interpretreportpb.InterpretReport) error {
	_, err := h.interpretReportClient.SaveInterpretReport(
		ctx,
		interpretReport.AnswerSheetId,
		interpretReport.MedicalScaleCode,
		interpretReport.Title,
		interpretReport.Description,
		interpretReport.InterpretItems,
	)
	if err != nil {
		return err
	}
	return nil
}
