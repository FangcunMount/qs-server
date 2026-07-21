// Package interpretationassets holds presentation/report assets for Interpretation.
// Decision rules and OutcomeCode determination belong in package decision (MC-R016).
package interpretationassets

// OutcomePresentation is display copy keyed by OutcomeCode.
type OutcomePresentation struct {
	OutcomeCode string
	Title       string
	Summary     string
	Description string
}

// TypeProfilePresentation is typology profile display copy.
type TypeProfilePresentation struct {
	OutcomeCode string
	Pattern     string
	Traits      []string
	Strengths   []string
	Weaknesses  []string
	Suggestions []string
	ImageURL    string
	Image       string
	Rarity      RarityPresentation
	IsSpecial   bool
	Trigger     string
	Commentary  string
}

// RarityPresentation is typology rarity display copy (MC-R017 batch 3).
type RarityPresentation struct {
	Percent float64
	Label   string
	OneInX  int
}

// ReportSection is one report assembly instruction.
type ReportSection struct {
	Code            string
	Title           string
	SourceRefs      []string
	Kind            string
	AdapterKey      string
	TemplateID      string
	TemplateVersion string
	CategoryLabel   string
}

// ReportSpec is the Interpretation-facing report map.
type ReportSpec struct {
	Sections []ReportSection
}

// Assets is the logical InterpretationAssets projected from DefinitionV2.
type Assets struct {
	Outcomes   []OutcomePresentation
	Profiles   []TypeProfilePresentation
	ReportSpec ReportSpec
}

// IsMaterialized reports whether presentation assets are present (MC-R016).
func (a Assets) IsMaterialized() bool {
	return len(a.Outcomes) > 0 || len(a.Profiles) > 0 || len(a.ReportSpec.Sections) > 0
}

// FindOutcome returns presentation for an OutcomeCode.
func (a Assets) FindOutcome(code string) (OutcomePresentation, bool) {
	for _, item := range a.Outcomes {
		if item.OutcomeCode == code {
			return item, true
		}
	}
	return OutcomePresentation{}, false
}

// FindProfile returns typology profile presentation for an OutcomeCode.
func (a Assets) FindProfile(code string) (TypeProfilePresentation, bool) {
	for _, item := range a.Profiles {
		if item.OutcomeCode == code {
			return item, true
		}
	}
	return TypeProfilePresentation{}, false
}
