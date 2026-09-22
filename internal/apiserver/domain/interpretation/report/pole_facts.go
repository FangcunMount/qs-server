package report

import (
	"fmt"
	"math"
)

const PoleFactsSchema = "mbti-pole-facts/v1"

// PoleFacts preserves authoritative values; Strength is a preference strength,
// never confidence. A nil block means legacy/missing facts, not zero strength.
type PoleFacts struct {
	SchemaVersion    string
	LeftPole         string
	RightPole        string
	Preference       string
	Strength         float64
	MinScore         float64
	MaxScore         float64
	Threshold        float64
	CompositionOrder int
}

func (p PoleFacts) Validate(raw float64) error {
	if p.SchemaVersion != PoleFactsSchema || p.LeftPole == "" || p.RightPole == "" || p.LeftPole == p.RightPole || (p.Preference != p.LeftPole && p.Preference != p.RightPole) {
		return fmt.Errorf("invalid pole facts identity")
	}
	for _, n := range []float64{raw, p.Strength, p.MinScore, p.MaxScore, p.Threshold} {
		if math.IsNaN(n) || math.IsInf(n, 0) {
			return fmt.Errorf("nonfinite pole fact")
		}
	}
	if p.Strength < 0 || p.Strength > 100 || p.MinScore >= p.Threshold || p.Threshold >= p.MaxScore || raw < p.MinScore || raw > p.MaxScore || p.CompositionOrder < 1 || p.CompositionOrder > 4 {
		return fmt.Errorf("invalid pole fact bounds")
	}
	return nil
}

func (d DimensionInterpret) WithPoleFacts(facts *PoleFacts) DimensionInterpret {
	d.poleFacts = clonePoleFacts(facts)
	return d
}

func (d DimensionInterpret) PoleFacts() *PoleFacts { return clonePoleFacts(d.poleFacts) }

func clonePoleFacts(p *PoleFacts) *PoleFacts {
	if p == nil {
		return nil
	}
	copy := *p
	return &copy
}

// ValidateMBTIPoleFacts checks the optional extension even on persisted reads.
// Legacy reports without it retain their existing compatibility behavior.
func ValidateMBTIPoleFacts(content Content) error {
	count := 0
	for _, d := range content.Dimensions {
		if d.poleFacts != nil {
			count++
		}
	}
	if count == 0 {
		return nil
	}
	if count != 4 || len(content.Dimensions) != 4 || content.Model.Kind != "typology" || content.Model.Code != "MBTI_OEJTS" || content.Model.Version != "v64-report-202608-v1" || content.Model.Algorithm != "personality_typology" || content.ModelExtra == nil || content.ModelExtra.IsSpecial {
		return fmt.Errorf("MBTI pole facts report identity mismatch")
	}
	codes := []string{"EI", "SN", "TF", "JP"}
	left := []string{"I", "S", "F", "J"}
	right := []string{"E", "N", "T", "P"}
	preferences := make([]byte, 4)
	for _, d := range content.Dimensions {
		p := d.poleFacts
		if err := p.Validate(d.RawScore()); err != nil {
			return err
		}
		i := p.CompositionOrder - 1
		if preferences[i] != 0 || d.Code().String() != codes[i] || d.Name() == "" || p.LeftPole != left[i] || p.RightPole != right[i] {
			return fmt.Errorf("MBTI pole facts axes mismatch")
		}
		preferences[i] = p.Preference[0]
	}
	if string(preferences) != content.ModelExtra.TypeCode {
		return fmt.Errorf("MBTI type and pole preferences mismatch")
	}
	return nil
}
