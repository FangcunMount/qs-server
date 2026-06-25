package sbti

import port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"

type DimensionResult struct {
	Code     string  `json:"code"`
	Name     string  `json:"name"`
	Model    string  `json:"model"`
	RawScore float64 `json:"raw_score"`
	Level    string  `json:"level"`
}

type ResultDetail struct {
	TypeCode       string                   `json:"type_code"`
	TypeName       string                   `json:"type_name"`
	OneLiner       string                   `json:"one_liner"`
	Pattern        string                   `json:"pattern"`
	Similarity     float64                  `json:"similarity"`
	ImageURL       string                   `json:"image_url"`
	Rarity         port.SBTIRaritySnapshot  `json:"rarity"`
	Dimensions     []DimensionResult        `json:"dimensions"`
	Outcome        port.SBTIOutcomeSnapshot `json:"outcome"`
	Source         port.SBTISourceSnapshot  `json:"source"`
	SpecialTrigger string                   `json:"special_trigger,omitempty"`
}
