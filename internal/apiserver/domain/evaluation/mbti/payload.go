package mbti

import "fmt"

func ResultDetailFromPayload(payload any) (ResultDetail, error) {
	switch detail := payload.(type) {
	case ResultDetail:
		return detail, nil
	case *ResultDetail:
		if detail == nil {
			return ResultDetail{}, fmt.Errorf("mbti result detail is nil")
		}
		return *detail, nil
	default:
		return ResultDetail{}, fmt.Errorf("unsupported mbti result detail payload: %T", payload)
	}
}
