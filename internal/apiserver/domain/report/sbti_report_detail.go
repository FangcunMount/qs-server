package report

import rulesetsbti "github.com/FangcunMount/qs-server/internal/apiserver/domain/ruleset/sbti"

type SBTIDimensionReport struct {
	Code     string
	Name     string
	Model    string
	RawScore float64
	Level    string
}

type SBTIReportDetail struct {
	TypeCode       string
	TypeName       string
	OneLiner       string
	Pattern        string
	Similarity     float64
	ImageURL       string
	Rarity         rulesetsbti.RaritySnapshot
	Dimensions     []SBTIDimensionReport
	Outcome        rulesetsbti.OutcomeSnapshot
	Source         rulesetsbti.SourceSnapshot
	SpecialTrigger string
}
