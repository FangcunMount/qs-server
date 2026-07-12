package presentation

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
)

func TestPresenterAppliesAudienceVisibility(t *testing.T) {
	presenter := Presenter{}
	participant, err := presenter.Allows(policy.AudienceParticipant, SectionModelExtra)
	if err != nil || !participant {
		t.Fatalf("participant=%#v err=%v", participant, err)
	}
	clinician, err := presenter.Allows(policy.AudienceClinician, SectionModelExtra)
	if err != nil || clinician {
		t.Fatalf("clinician=%#v err=%v", clinician, err)
	}
	admin, err := presenter.Allows(policy.AudienceAdmin, SectionModelExtra)
	if err != nil || !admin {
		t.Fatalf("admin=%#v err=%v", admin, err)
	}
}
