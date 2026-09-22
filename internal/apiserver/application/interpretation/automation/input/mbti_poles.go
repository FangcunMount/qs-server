package input

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/typology/patterns"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

func attachFrozenMBTIPoles(dimensions []patterns.PersonalityTypeDimensionReport, execution *evaluationfact.Execution, assets *evaluationinput.InputSnapshot, typeCode string) error {
	if assets == nil || assets.MBTIPoles == nil {
		return nil
	} // No latest-model fallback for old Outcomes.
	if err := assets.MBTIPoles.Validate(); err != nil {
		return err
	}
	if len(dimensions) != 4 || len(execution.Dimensions) != 4 {
		return fmt.Errorf("MBTI requires four scored pole facts")
	}
	seen := map[string]bool{}
	composed := make([]byte, 4)
	for i, d := range execution.Dimensions {
		if seen[d.Code] || d.Score == nil || d.Strength == nil || d.Score.Kind != evaluationfact.ScoreKindRawTotal {
			return fmt.Errorf("MBTI pole score or strength is missing or duplicated")
		}
		seen[d.Code] = true
		found := false
		for order, axis := range assets.MBTIPoles.Axes {
			if axis.Code != d.Code {
				continue
			}
			found = true
			if (d.LeftPole != "" && d.LeftPole != axis.LeftPole) || (d.RightPole != "" && d.RightPole != axis.RightPole) {
				return fmt.Errorf("MBTI outcome poles conflict with frozen model")
			}
			facts := &report.PoleFacts{SchemaVersion: report.PoleFactsSchema, LeftPole: axis.LeftPole, RightPole: axis.RightPole, Preference: d.Preference, Strength: *d.Strength, MinScore: axis.MinScore, MaxScore: axis.MaxScore, Threshold: axis.Threshold, CompositionOrder: order + 1}
			if err := facts.Validate(d.Score.Value); err != nil {
				return err
			}
			composed[order] = d.Preference[0]
			dimensions[i].Name = axis.Name
			dimensions[i].LeftPole, dimensions[i].RightPole = axis.LeftPole, axis.RightPole
			dimensions[i].PoleFacts = facts
		}
		if !found {
			return fmt.Errorf("MBTI outcome axis is not in frozen model")
		}
	}
	if string(composed) != typeCode {
		return fmt.Errorf("MBTI outcome type conflicts with pole preferences")
	}
	return nil
}
