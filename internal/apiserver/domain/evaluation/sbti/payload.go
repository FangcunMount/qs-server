package sbti

import "fmt"

func ResultDetailFromPayload(payload any) (ResultDetail, error) {
	switch detail := payload.(type) {
	case ResultDetail:
		return detail, nil
	case *ResultDetail:
		if detail == nil {
			return ResultDetail{}, fmt.Errorf("sbti result detail is nil")
		}
		return *detail, nil
	default:
		return ResultDetail{}, fmt.Errorf("unsupported sbti result detail payload: %T", payload)
	}
}
