// Package presentation applies audience visibility rules to one canonical,
// immutable Interpretation report.
package presentation

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
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
