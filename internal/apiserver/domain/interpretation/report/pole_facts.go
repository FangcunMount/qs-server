package report

import (
	"fmt"
	"math"

	"github.com/FangcunMount/qs-server/internal/pkg/mbticontract"
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
	model, supported := mbticontract.Lookup(content.Model.Code, content.Model.Version)
	if !supported || count != 4 || len(content.Dimensions) != 4 || content.Model.Kind != "typology" || content.Model.Algorithm != "personality_typology" || content.ModelExtra == nil || content.ModelExtra.IsSpecial {
		return fmt.Errorf("MBTI pole facts report identity mismatch")
	}
	preferences := make([]byte, 4)
	for _, d := range content.Dimensions {
		p := d.poleFacts
		if err := p.Validate(d.RawScore()); err != nil {
			return err
		}
		i := p.CompositionOrder - 1
		if preferences[i] != 0 || d.Code().String() != model.Axes[i].Code || d.Name() == "" || p.LeftPole != model.Axes[i].Left || p.RightPole != model.Axes[i].Right {
			return fmt.Errorf("MBTI pole facts axes mismatch")
		}
		if content.Model.Code == "MBTI_FC_93" && (p.MinScore != model.Axes[i].Min || p.MaxScore != model.Axes[i].Max || p.Threshold != model.Axes[i].Threshold) {
			return fmt.Errorf("exploration pole bounds mismatch")
		}
		preferences[i] = p.Preference[0]
	}
	if string(preferences) != content.ModelExtra.TypeCode {
		return fmt.Errorf("MBTI type and pole preferences mismatch")
	}
	return nil
}
