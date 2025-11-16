package interpretion

import (
	"fmt"
	"math"

	"github.com/FangcunMount/component-base/pkg/log"
	interpretreportpb "github.com/FangcunMount/qs-server/internal/apiserver/interface/grpc/proto/interpret-report"
	medicalscalepb "github.com/FangcunMount/qs-server/internal/apiserver/interface/grpc/proto/medical-scale"
)

// InterpretReportContentGenerator 解读报告内容生成器
type InterpretReportContentGenerator struct{}

// NewInterpretReportContentGenerator 创建解读报告内容生成器
func NewInterpretReportContentGenerator() *InterpretReportContentGenerator {
	return &InterpretReportContentGenerator{}
}

// GenerateInterpretContent 为解读报告生成解读内容
// 根据医学量表的因子解读规则和解读报告的因子分，为每个因子生成解读文案
func (g *InterpretReportContentGenerator) GenerateInterpretContent(
	interpretReport *interpretreportpb.InterpretReport,
	medicalScale *medicalscalepb.MedicalScale,
) error {
	log.Infof("开始生成解读报告内容，因子数量: %d", len(interpretReport.InterpretItems))

	// 创建因子映射，便于快速查找
	factorMap := make(map[string]*medicalscalepb.Factor)
	for _, factor := range medicalScale.Factors {
		factorMap[factor.Code] = factor
	}

	// 为每个解读项生成内容
	for _, interpretItem := range interpretReport.InterpretItems {
		factor := factorMap[interpretItem.FactorCode]
		if factor == nil {
			log.Warnf("未找到因子，代码: %s", interpretItem.FactorCode)
			continue
		}

		// 生成解读内容
		content, err := g.generateFactorContent(factor, interpretItem.Score)
		if err != nil {
			log.Errorf("生成因子解读内容失败，因子: %s, 错误: %v", factor.Code, err)
			continue
		}

		// 更新解读项的内容
		interpretItem.Content = content
		log.Infof("因子 %s 解读内容生成完成: %s", factor.Code, content)
	}

	log.Infof("解读报告内容生成完成")
	return nil
}

// generateFactorContent 为单个因子生成解读内容
func (g *InterpretReportContentGenerator) generateFactorContent(
	factor *medicalscalepb.Factor,
	score float64,
) (string, error) {
	// 检查因子是否有解读规则
	if len(factor.InterpretationRules) == 0 {
		log.Warnf("因子 %s 没有解读规则，使用默认内容", factor.Code)
		return g.generateDefaultContent(factor, score), nil
	}

	// 根据分数找到匹配的解读规则
	matchedRule := g.findMatchingInterpretRule(factor.InterpretationRules, score)
	if matchedRule == nil {
		log.Warnf("因子 %s 分数 %.2f 没有匹配的解读规则，使用默认内容", factor.Code, score)
		return g.generateDefaultContent(factor, score), nil
	}

	// 返回匹配的解读内容
	return matchedRule.Content, nil
}

// findMatchingInterpretRule 根据分数找到匹配的解读规则
// 使用左闭右开区间 [min, max) 进行匹配
func (g *InterpretReportContentGenerator) findMatchingInterpretRule(
	rules []*medicalscalepb.InterpretationRule,
	score float64,
) *medicalscalepb.InterpretationRule {
	for _, rule := range rules {
		if rule.ScoreRange == nil {
			continue
		}

		// 检查分数是否在范围内（左闭右开区间）
		if score >= rule.ScoreRange.MinScore && score < rule.ScoreRange.MaxScore {
			return rule
		}
	}
	return nil
}

// generateDefaultContent 生成默认的解读内容
func (g *InterpretReportContentGenerator) generateDefaultContent(
	factor *medicalscalepb.Factor,
	score float64,
) string {
	// 检查分数是否有效
	if math.IsNaN(score) || math.IsInf(score, 0) {
		return fmt.Sprintf("因子 %s (%s) 的评估结果：分数无效", factor.Title, factor.Code)
	}

	// 根据分数范围生成不同的默认内容
	if score == 0 {
		return fmt.Sprintf("因子 %s (%s) 的评估结果：得分为0，属于正常范畴", factor.Title, factor.Code)
	} else if score < 5 {
		return fmt.Sprintf("因子 %s (%s) 的评估结果：得分%.2f，属于轻度异常", factor.Title, factor.Code, score)
	} else if score < 10 {
		return fmt.Sprintf("因子 %s (%s) 的评估结果：得分%.2f，属于中度异常，建议关注", factor.Title, factor.Code, score)
	} else {
		return fmt.Sprintf("因子 %s (%s) 的评估结果：得分%.2f，属于重度异常，建议及时干预", factor.Title, factor.Code, score)
	}
}

// ValidateInterpretContent 验证解读内容的完整性
func (g *InterpretReportContentGenerator) ValidateInterpretContent(
	interpretReport *interpretreportpb.InterpretReport,
) error {
	for _, item := range interpretReport.InterpretItems {
		if item.Content == "" {
			return fmt.Errorf("因子 %s 的解读内容为空", item.FactorCode)
		}
	}
	return nil
}

// GenerateSummaryContent 生成解读报告的总结合内容
func (g *InterpretReportContentGenerator) GenerateSummaryContent(
	interpretReport *interpretreportpb.InterpretReport,
) string {
	if len(interpretReport.InterpretItems) == 0 {
		return "暂无解读内容"
	}

	// 统计各因子的情况
	var normalCount, mildCount, moderateCount, severeCount int
	var totalScore float64

	for _, item := range interpretReport.InterpretItems {
		totalScore += item.Score

		// 根据分数范围分类
		if item.Score == 0 {
			normalCount++
		} else if item.Score < 5 {
			mildCount++
		} else if item.Score < 10 {
			moderateCount++
		} else {
			severeCount++
		}
	}

	// 生成总结内容
	summary := fmt.Sprintf("本次评估共包含%d个因子，总分%.2f。", len(interpretReport.InterpretItems), totalScore)

	if normalCount > 0 {
		summary += fmt.Sprintf("其中%d个因子属于正常范畴，", normalCount)
	}
	if mildCount > 0 {
		summary += fmt.Sprintf("%d个因子属于轻度异常，", mildCount)
	}
	if moderateCount > 0 {
		summary += fmt.Sprintf("%d个因子属于中度异常，", moderateCount)
	}
	if severeCount > 0 {
		summary += fmt.Sprintf("%d个因子属于重度异常，", severeCount)
	}

	// 根据整体情况给出建议
	if severeCount > 0 {
		summary += "建议及时就医，寻求专业医生的帮助。"
	} else if moderateCount > 0 {
		summary += "建议定期评估，关注相关症状的发展。"
	} else if mildCount > 0 {
		summary += "建议适当关注，保持健康的生活方式。"
	} else {
		summary += "整体评估结果良好，建议继续保持。"
	}

	return summary
}
