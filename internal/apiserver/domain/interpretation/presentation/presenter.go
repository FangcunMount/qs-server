// Package presentation applies audience visibility rules to one canonical,
// immutable Interpretation report.
package presentation

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
)

type Presenter struct{}

type Section string

const SectionModelExtra Section = "model_extra"

func (Presenter) Allows(audience policy.Audience, section Section) (bool, error) {
	if section != SectionModelExtra {
		return false, fmt.Errorf("unsupported report section %q", section)
	}
	switch audience {
	case policy.AudienceParticipant, policy.AudienceAdmin:
		return true, nil
	case policy.AudienceClinician:
		return false, nil
	default:
		return false, fmt.Errorf("unsupported report audience %q", audience)
	}
}

func (Presenter) Present(content report.Content, audience policy.Audience) (report.Content, error) {
	projected := report.NewDraft(content).Content()
	visible, err := (Presenter{}).Allows(audience, SectionModelExtra)
	if err != nil {
		return report.Content{}, err
	}
	if !visible {
		projected.ModelExtra = nil
	}
	return projected, nil
}
