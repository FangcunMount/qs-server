package legacy

import (
	"fmt"

	outcometypology "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome/typology"
)

// PersonalityTypeDetailForReport normalizes generic or legacy personality payloads for report building.
func PersonalityTypeDetailForReport(payload any) (outcometypology.PersonalityTypeDetail, error) {
	if detail, err := outcometypology.PersonalityTypeDetailFromPayload(payload); err == nil {
		return detail, nil
	}
	if detail, err := MBTIResultDetailFromPayload(payload); err == nil {
		return PersonalityTypeDetailFromMBTI(detail), nil
	}
	if detail, err := SBTIResultDetailFromPayload(payload); err == nil {
		return PersonalityTypeDetailFromSBTI(detail), nil
	}
	return outcometypology.PersonalityTypeDetail{}, fmt.Errorf("unsupported personality type detail payload")
}

// TraitProfileDetailForReport normalizes generic or legacy trait profile payloads for report building.
func TraitProfileDetailForReport(payload any) (outcometypology.TraitProfileDetail, error) {
	if detail, err := outcometypology.TraitProfileDetailFromPayload(payload); err == nil {
		return detail, nil
	}
	if detail, err := BigFiveResultDetailFromPayload(payload); err == nil {
		return TraitProfileDetailFromBigFive(detail), nil
	}
	return outcometypology.TraitProfileDetail{}, fmt.Errorf("unsupported trait profile detail payload")
}
