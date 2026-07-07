package patterns

import modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/personality/typology"

type BigFiveTraitResult struct {
	Code     string  `json:"code"`
	Name     string  `json:"name"`
	RawScore float64 `json:"raw_score"`
}

type BigFiveResultDetail struct {
	Traits []BigFiveTraitResult `json:"traits"`
	Source modeltypology.Source `json:"source"`
}
