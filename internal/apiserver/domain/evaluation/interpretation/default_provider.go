package interpretation

import "fmt"

// ==================== 默认解读提供者 ====================

// DefaultInterpretationProvider 默认解读提供者
// 当没有配置解读规则时，使用默认模版生成解读
// 设计原则：提供通用的、符合医学常规的默认解读
type DefaultInterpretationProvider struct{}

// NewDefaultInterpretationProvider 创建默认解读提供者
func NewDefaultInterpretationProvider() *DefaultInterpretationProvider {
	return &DefaultInterpretationProvider{}
}

// ProvideFactor 为因子提供默认解读
// factorName: 因子名称（用于生成可读的描述）
// score: 因子得分
// riskLevel: 风险等级
// 返回：解读结果
func (p *DefaultInterpretationProvider) ProvideFactor(
	factorName string,
	score float64,
	riskLevel RiskLevel,
) *InterpretResult {
	var label, description, suggestion string

	switch riskLevel {
	case RiskLevelSevere:
		label = "严重异常"
		description = fmt.Sprintf("%s得分%.1f分，处于严重异常水平", factorName, score)
		suggestion = "建议立即寻求专业帮助，进行进一步评估"

	case RiskLevelHigh:
		label = "较高风险"
		description = fmt.Sprintf("%s得分%.1f分，处于较高风险水平", factorName, score)
		suggestion = "建议尽快咨询专业人员，了解更多信息"

	case RiskLevelMedium:
		label = "中等水平"
		description = fmt.Sprintf("%s得分%.1f分，处于中等水平", factorName, score)
		suggestion = "建议关注相关方面，适当调整生活方式"

	case RiskLevelLow:
		label = "正常偏低"
		description = fmt.Sprintf("%s得分%.1f分，处于正常偏低水平", factorName, score)
		suggestion = "整体情况良好，保持当前状态"

	default: // RiskLevelNone
		label = "正常"
		description = fmt.Sprintf("%s得分%.1f分，处于正常水平", factorName, score)
		suggestion = "状态良好，继续保持"
	}

	return &InterpretResult{
		Score:       score,
		RiskLevel:   riskLevel,
		Label:       label,
		Description: description,
		Suggestion:  suggestion,
	}
}

// ProvideOverall 提供整体默认解读
// totalScore: 总分
// riskLevel: 风险等级
// 返回：解读结果
func (p *DefaultInterpretationProvider) ProvideOverall(
	totalScore float64,
	riskLevel RiskLevel,
) *InterpretResult {
	var label, description, suggestion string

	switch riskLevel {
	case RiskLevelSevere:
		label = "严重问题"
		description = "测评结果显示存在严重问题，需要立即关注"
		suggestion = "强烈建议尽快寻求专业帮助，进行全面评估和干预"

	case RiskLevelHigh:
		label = "较高风险"
		description = "测评结果显示存在较高风险，需要重点关注"
		suggestion = "建议尽快咨询专业人员，获取更详细的评估和指导"

	case RiskLevelMedium:
		label = "一定风险"
		description = "测评结果显示存在一定风险，需要适度关注"
		suggestion = "建议关注相关方面的变化，必要时寻求专业帮助"

	case RiskLevelLow:
		label = "基本良好"
		description = "测评结果显示整体情况良好，少数方面需要注意"
		suggestion = "保持健康的生活方式，定期进行自我检查"

	default: // RiskLevelNone
		label = "正常"
		description = "测评已完成，整体情况良好"
		suggestion = "保持健康的生活方式"
	}

	return &InterpretResult{
		Score:       totalScore,
		RiskLevel:   riskLevel,
		Label:       label,
		Description: description,
		Suggestion:  suggestion,
	}
}

// ProvideFactorWithTemplate 使用自定义模版提供因子解读
// template: 描述模版，使用 %s (因子名) 和 %.1f (得分) 作为占位符
// factorName: 因子名称
// score: 因子得分
// riskLevel: 风险等级
// suggestion: 建议（可选，如果为空则使用默认建议）
func (p *DefaultInterpretationProvider) ProvideFactorWithTemplate(
	template string,
	factorName string,
	score float64,
	riskLevel RiskLevel,
	suggestion string,
) *InterpretResult {
	description := fmt.Sprintf(template, factorName, score)

	// 如果没有提供建议，使用默认建议
	if suggestion == "" {
		defaultResult := p.ProvideFactor(factorName, score, riskLevel)
		suggestion = defaultResult.Suggestion
	}

	return &InterpretResult{
		Score:       score,
		RiskLevel:   riskLevel,
		Label:       string(riskLevel),
		Description: description,
		Suggestion:  suggestion,
	}
}

// ==================== 默认提供者实例 ====================

// 默认解读提供者（单例）
var defaultProvider = NewDefaultInterpretationProvider()

// GetDefaultProvider 获取默认解读提供者
func GetDefaultProvider() *DefaultInterpretationProvider {
	return defaultProvider
}

// ==================== 便捷函数 ====================

// ProvideDefaultFactor 使用默认提供者生成因子解读（便捷函数）
func ProvideDefaultFactor(factorName string, score float64, riskLevel RiskLevel) *InterpretResult {
	return defaultProvider.ProvideFactor(factorName, score, riskLevel)
}

// ProvideDefaultOverall 使用默认提供者生成整体解读（便捷函数）
func ProvideDefaultOverall(totalScore float64, riskLevel RiskLevel) *InterpretResult {
	return defaultProvider.ProvideOverall(totalScore, riskLevel)
}
