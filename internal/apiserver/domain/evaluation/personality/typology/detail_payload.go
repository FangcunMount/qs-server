package typology

import "fmt"

func MBTIResultDetailFromPayload(payload any) (MBTIResultDetail, error) {
	switch detail := payload.(type) {
	case MBTIResultDetail:
		return detail, nil
	case *MBTIResultDetail:
		if detail == nil {
			return MBTIResultDetail{}, fmt.Errorf("mbti result detail is nil")
		}
		return *detail, nil
	default:
		return MBTIResultDetail{}, fmt.Errorf("unsupported mbti result detail payload: %T", payload)
	}
}

func SBTIResultDetailFromPayload(payload any) (SBTIResultDetail, error) {
	switch detail := payload.(type) {
	case SBTIResultDetail:
		return detail, nil
	case *SBTIResultDetail:
		if detail == nil {
			return SBTIResultDetail{}, fmt.Errorf("sbti result detail is nil")
		}
		return *detail, nil
	default:
		return SBTIResultDetail{}, fmt.Errorf("unsupported sbti result detail payload: %T", payload)
	}
}
