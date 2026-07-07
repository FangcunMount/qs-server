package legacy

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/personality/typology"
)

// 默认AlgorithmAliases 返回内置 类型学算法 别名 用于 旧版 路由。
func DefaultAlgorithmAliases() []modelcatalog.Algorithm {
	return []modelcatalog.Algorithm{
		modelcatalog.AlgorithmMBTI,
		modelcatalog.AlgorithmSBTI,
		modelcatalog.AlgorithmBigFive,
	}
}

// CategoryLabelFor 解析display label 用于 旧版 类型学算法 别名。
func CategoryLabelFor(algorithm modelcatalog.Algorithm) string {
	switch algorithm {
	case modelcatalog.AlgorithmSBTI:
		return "SBTI"
	case modelcatalog.AlgorithmBigFive:
		return "Big Five"
	default:
		return "MBTI"
	}
}

// ReportSpecForAlgorithm 推导report spec 从 旧版 算法 identifier。
func ReportSpecForAlgorithm(algorithm modelcatalog.Algorithm) modeltypology.ReportSpec {
	return modeltypology.LegacyReportSpecFromAlgorithm(algorithm)
}

// OutcomeMappingForAlgorithm 推导结果 mapping 从 旧版 算法 identifier。
func OutcomeMappingForAlgorithm(algorithm modelcatalog.Algorithm) modeltypology.OutcomeMappingSpec {
	return modeltypology.LegacyOutcomeMappingFromAlgorithm(algorithm)
}
