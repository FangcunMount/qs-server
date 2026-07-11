package evaluationtypologylegacy

import legacy "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/typology/legacy"

type MBTIResultDetail = legacy.MBTIResultDetail

var (
	MBTIResultDetailFromPayload    = legacy.MBTIResultDetailFromPayload
	PersonalityTypeDetailFromMBTI  = legacy.PersonalityTypeDetailFromMBTI
	SBTIResultDetailFromPayload    = legacy.SBTIResultDetailFromPayload
	PersonalityTypeDetailFromSBTI  = legacy.PersonalityTypeDetailFromSBTI
	BigFiveResultDetailFromPayload = legacy.BigFiveResultDetailFromPayload
	TraitProfileDetailFromBigFive  = legacy.TraitProfileDetailFromBigFive
)
