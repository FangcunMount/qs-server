package report

// ModelExtra 解释模型扩展信息（可选，SBTI 等人格类测评使用）。
type ModelExtra struct {
	Kind           string       `json:"kind,omitempty"`
	TypeCode       string       `json:"type_code,omitempty"`
	TypeName       string       `json:"type_name,omitempty"`
	OneLiner       string       `json:"one_liner,omitempty"`
	ImageURL       string       `json:"image_url,omitempty"`
	MatchPercent   float64      `json:"match_percent,omitempty"`
	IsSpecial      bool         `json:"is_special,omitempty"`
	SpecialTrigger string       `json:"special_trigger,omitempty"`
	Rarity         *ModelRarity `json:"rarity,omitempty"`
	Commentary     string       `json:"commentary,omitempty"`
}

// ModelRarity 理论稀有度。
type ModelRarity struct {
	Percent float64 `json:"percent,omitempty"`
	Label   string  `json:"label,omitempty"`
	OneInX  int     `json:"one_in_x,omitempty"`
}

func (e *ModelExtra) IsEmpty() bool {
	if e == nil {
		return true
	}
	return e.Kind == "" &&
		e.TypeCode == "" &&
		e.TypeName == "" &&
		e.OneLiner == "" &&
		e.ImageURL == "" &&
		e.MatchPercent == 0 &&
		!e.IsSpecial &&
		e.SpecialTrigger == "" &&
		e.Commentary == "" &&
		(e.Rarity == nil || (e.Rarity.Percent == 0 && e.Rarity.Label == "" && e.Rarity.OneInX == 0))
}
