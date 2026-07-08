package evaluation

// personalityModelKind 是人格/类型学模型的领域 kind 数据值。
// 注意：这是历史数据值（KindPersonality），非接口命名，本轮不迁移。
const personalityModelKind = "personality"

// scaleCodeFromModel 返回量表类模型的 code；人格类模型无量表码，返回空串。
func scaleCodeFromModel(model ModelIdentityResponse) string {
	if model.Kind == personalityModelKind {
		return ""
	}
	return model.Code
}

// scaleNameFromModel 返回量表类模型的名称；人格类模型无量表名，返回空串。
func scaleNameFromModel(model ModelIdentityResponse) string {
	if model.Kind == personalityModelKind {
		return ""
	}
	return model.Title
}

var scaleRiskLevelCodes = map[string]struct{}{
	"none": {}, "low": {}, "medium": {}, "high": {}, "severe": {},
}

func isScaleRiskLevelCode(code string) bool {
	_, ok := scaleRiskLevelCodes[code]
	return ok
}

// OutcomeTotalScore 从 outcome 主分投影提取量表总分。
func OutcomeTotalScore(score *ScoreValueResponse) float64 {
	if score == nil {
		return 0
	}
	return score.Value
}

// OutcomeRiskLevel 从 outcome 等级投影提取量表风险等级码。
func OutcomeRiskLevel(level *ResultLevelResponse) string {
	if level == nil {
		return ""
	}
	if isScaleRiskLevelCode(level.Code) {
		return level.Code
	}
	if isScaleRiskLevelCode(level.Severity) {
		return level.Severity
	}
	return ""
}
