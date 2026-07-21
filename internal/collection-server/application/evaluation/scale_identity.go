package evaluation

const (
	// typologyModelKind 是当前类型学模型的规范 Kind。
	typologyModelKind = "typology"
)

// IsTypologyModel reports whether an assessment model uses the canonical typology kind.
func IsTypologyModel(model ModelIdentityResponse) bool {
	return model.Kind == typologyModelKind
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
