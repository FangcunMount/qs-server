package evaluation

const (
	// typologyModelKind 是当前类型学模型的规范 Kind。
	typologyModelKind = "typology"
	// personalityModelKind 是迁移前类型学记录的历史 Kind。
	personalityModelKind = "personality"
)

// IsTypologyModel reports whether an assessment model belongs to the typology
// facade. Canonical records use kind=typology; legacy personality records are
// retained for read compatibility while old assessments are still present.
func IsTypologyModel(model ModelIdentityResponse) bool {
	switch model.Kind {
	case typologyModelKind:
		return true
	case personalityModelKind:
		return model.SubKind == "" || model.SubKind == "typology"
	default:
		return false
	}
}

// scaleCodeFromModel 返回量表类模型的 code；人格类模型无量表码，返回空串。
func scaleCodeFromModel(model ModelIdentityResponse) string {
	if IsTypologyModel(model) {
		return ""
	}
	return model.Code
}

// scaleNameFromModel 返回量表类模型的名称；人格类模型无量表名，返回空串。
func scaleNameFromModel(model ModelIdentityResponse) string {
	if IsTypologyModel(model) {
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
