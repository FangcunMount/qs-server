package answersheet

// ==================== 答案值适配器 ====================

// answerValueAdapter exposes AnswerValue through the scoring value surface.
type answerValueAdapter struct {
	value AnswerValue
}

// NewScorableValue 创建可计分值适配器
func NewScorableValue(value AnswerValue) *answerValueAdapter {
	return &answerValueAdapter{value: value}
}

func (a *answerValueAdapter) IsEmpty() bool {
	return a.value == nil || a.value.Raw() == nil
}

func (a *answerValueAdapter) AsSingleSelection() (string, bool) {
	if a.value == nil {
		return "", false
	}
	raw := a.value.Raw()
	if str, ok := raw.(string); ok {
		return str, true
	}
	return "", false
}

func (a *answerValueAdapter) AsMultipleSelections() ([]string, bool) {
	if a.value == nil {
		return nil, false
	}
	raw := a.value.Raw()

	switch v := raw.(type) {
	case []string:
		return v, true
	case []interface{}:
		// 处理从JSON反序列化的情况
		result := make([]string, 0, len(v))
		for _, item := range v {
			if str, ok := item.(string); ok {
				result = append(result, str)
			}
		}
		return result, len(result) > 0
	}
	return nil, false
}

func (a *answerValueAdapter) AsNumber() (float64, bool) {
	if a.value == nil {
		return 0, false
	}
	raw := a.value.Raw()

	switch v := raw.(type) {
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case float64:
		return v, true
	}
	return 0, false
}

// ==================== 计分结果值对象 ====================

// ScoredAnswerSheet 已计分的答卷
type ScoredAnswerSheet struct {
	AnswerSheetID uint64
	TotalScore    float64
	ScoredAnswers []ScoredAnswer
}

// ScoredAnswer 已计分的答案
type ScoredAnswer struct {
	QuestionCode string
	Score        float64
	MaxScore     float64
}
