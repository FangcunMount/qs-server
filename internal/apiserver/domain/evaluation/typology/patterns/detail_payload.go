package patterns

import "fmt"

func PersonalityTypeDetailFromPayload(payload any) (PersonalityTypeDetail, error) {
	switch detail := payload.(type) {
	case PersonalityTypeDetail:
		return detail, nil
	case *PersonalityTypeDetail:
		if detail == nil {
			return PersonalityTypeDetail{}, fmt.Errorf("personality type detail is nil")
		}
		return *detail, nil
	default:
		return PersonalityTypeDetail{}, fmt.Errorf("unsupported personality type detail payload: %T", payload)
	}
}

func TraitProfileDetailFromPayload(payload any) (TraitProfileDetail, error) {
	switch detail := payload.(type) {
	case TraitProfileDetail:
		return detail, nil
	case *TraitProfileDetail:
		if detail == nil {
			return TraitProfileDetail{}, fmt.Errorf("trait profile detail is nil")
		}
		return *detail, nil
	default:
		return TraitProfileDetail{}, fmt.Errorf("unsupported trait profile detail payload: %T", payload)
	}
}

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

func BigFiveResultDetailFromPayload(payload any) (BigFiveResultDetail, error) {
	switch detail := payload.(type) {
	case BigFiveResultDetail:
		return detail, nil
	case *BigFiveResultDetail:
		if detail == nil {
			return BigFiveResultDetail{}, fmt.Errorf("bigfive result detail is nil")
		}
		return *detail, nil
	default:
		return BigFiveResultDetail{}, fmt.Errorf("unsupported bigfive result detail payload: %T", payload)
	}
}
