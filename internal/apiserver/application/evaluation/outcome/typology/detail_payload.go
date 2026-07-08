package typology

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
