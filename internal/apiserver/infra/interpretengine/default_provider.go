package interpretengine

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/port/interpretengine"
)

type DefaultProvider struct{}

func NewDefaultProvider() *DefaultProvider {
	return &DefaultProvider{}
}

func (p *DefaultProvider) ProvideFactor(factorName string, score float64, riskLevel string) *interpretengine.Result {
	var label, description, suggestion string

	switch riskLevel {
	case "severe":
		label = "严重异常"
		description = fmt.Sprintf("%s得分%.1f分，处于严重异常水平", factorName, score)
		suggestion = "建议立即寻求专业帮助，进行进一步评估"
	case "high":
		label = "较高风险"
		description = fmt.Sprintf("%s得分%.1f分，处于较高风险水平", factorName, score)
		suggestion = "建议尽快咨询专业人员，了解更多信息"
	case "medium":
		label = "中等水平"
		description = fmt.Sprintf("%s得分%.1f分，处于中等水平", factorName, score)
		suggestion = "建议关注相关方面，适当调整生活方式"
	case "low":
		label = "正常偏低"
		description = fmt.Sprintf("%s得分%.1f分，处于正常偏低水平", factorName, score)
		suggestion = "整体情况良好，保持当前状态"
	default:
		label = "正常"
		description = fmt.Sprintf("%s得分%.1f分，处于正常水平", factorName, score)
		suggestion = "状态良好，继续保持"
	}

	return &interpretengine.Result{
		Score:       score,
		RiskLevel:   riskLevel,
		Label:       label,
		Description: description,
		Suggestion:  suggestion,
	}
}

func (p *DefaultProvider) ProvideOverall(totalScore float64, riskLevel string) *interpretengine.Result {
	var label, description, suggestion string

	switch riskLevel {
	case "severe":
		label = "严重问题"
		description = "测评结果显示存在严重问题，需要立即关注"
		suggestion = "强烈建议尽快寻求专业帮助，进行全面评估和干预"
	case "high":
		label = "较高风险"
		description = "测评结果显示存在较高风险，需要重点关注"
		suggestion = "建议尽快咨询专业人员，获取更详细的评估和指导"
	case "medium":
		label = "一定风险"
		description = "测评结果显示存在一定风险，需要适度关注"
		suggestion = "建议关注相关方面的变化，必要时寻求专业帮助"
	case "low":
		label = "基本良好"
		description = "测评结果显示整体情况良好，少数方面需要注意"
		suggestion = "保持健康的生活方式，定期进行自我检查"
	default:
		label = "正常"
		description = "测评已完成，整体情况良好"
		suggestion = "保持健康的生活方式"
	}

	return &interpretengine.Result{
		Score:       totalScore,
		RiskLevel:   riskLevel,
		Label:       label,
		Description: description,
		Suggestion:  suggestion,
	}
}
