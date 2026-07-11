package typology

import (
	"fmt"

	outcometypology "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationtypology"
	typologylegacy "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationtypologylegacy"
)

// PersonalityTypeDetailForReport normalizes generic or legacy personality payloads for report building.
func personalityTypeDetailFromLegacyPayload(payload any) (outcometypology.PersonalityTypeDetail, error) {
	if detail, err := outcometypology.PersonalityTypeDetailFromPayload(payload); err == nil {
		return detail, nil
	}
	if detail, err := typologylegacy.MBTIResultDetailFromPayload(payload); err == nil {
		return typologylegacy.PersonalityTypeDetailFromMBTI(detail), nil
	}
	if detail, err := typologylegacy.SBTIResultDetailFromPayload(payload); err == nil {
		return typologylegacy.PersonalityTypeDetailFromSBTI(detail), nil
	}
	return outcometypology.PersonalityTypeDetail{}, fmt.Errorf("unsupported personality type detail payload")
}

// TraitProfileDetailForReport normalizes generic or legacy trait profile payloads for report building.
func traitProfileDetailFromLegacyPayload(payload any) (outcometypology.TraitProfileDetail, error) {
	if detail, err := outcometypology.TraitProfileDetailFromPayload(payload); err == nil {
		return detail, nil
	}
	if detail, err := typologylegacy.BigFiveResultDetailFromPayload(payload); err == nil {
		return typologylegacy.TraitProfileDetailFromBigFive(detail), nil
	}
	return outcometypology.TraitProfileDetail{}, fmt.Errorf("unsupported trait profile detail payload")
}
