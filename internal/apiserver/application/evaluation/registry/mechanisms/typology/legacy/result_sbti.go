package legacy

import (
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/personality/typology"
)

type SBTIDimensionResult struct {
	Code     string  `json:"code"`
	Name     string  `json:"name"`
	Model    string  `json:"model"`
	RawScore float64 `json:"raw_score"`
	Level    string  `json:"level"`
}

type SBTIResultDetail struct {
	TypeCode       string                          `json:"type_code"`
	TypeName       string                          `json:"type_name"`
	OneLiner       string                          `json:"one_liner"`
	Pattern        string                          `json:"pattern"`
	Similarity     float64                         `json:"similarity"`
	ImageURL       string                          `json:"image_url"`
	Rarity         modeltypology.SBTILegacyRarity  `json:"rarity"`
	Dimensions     []SBTIDimensionResult           `json:"dimensions"`
	Outcome        modeltypology.SBTILegacyOutcome `json:"outcome"`
	Source         modeltypology.SBTILegacySource  `json:"source"`
	SpecialTrigger string                          `json:"special_trigger,omitempty"`
}
