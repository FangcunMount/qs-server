package calculation

// extractQuestionCodeFromID 从ID中提取问题代码
func extractQuestionCodeFromID(id string) string {
	const prefix = "answer_"
	if len(id) > len(prefix) && id[:len(prefix)] == prefix {
		return id[len(prefix):]
	}
	return ""
}

// extractFactorCodeFromID 从ID中提取因子代码
func extractFactorCodeFromID(id string) string {
	const prefix = "factor_"
	if len(id) > len(prefix) && id[:len(prefix)] == prefix {
		return id[len(prefix):]
	}
	return ""
}
