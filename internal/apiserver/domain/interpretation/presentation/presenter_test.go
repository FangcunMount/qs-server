package presentation

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
)

func TestPresenterProjectsOneCanonicalReportByAudience(t *testing.T) {
	canonical := report.Content{Conclusion: "stable", ModelExtra: &report.ModelExtra{Kind: "personality"}}
	presenter := Presenter{}
	participant, err := presenter.Present(canonical, policy.AudienceParticipant)
	if err != nil || participant.ModelExtra == nil {
		t.Fatalf("participant=%#v err=%v", participant, err)
	}
	clinician, err := presenter.Present(canonical, policy.AudienceClinician)
	if err != nil || clinician.ModelExtra != nil {
		t.Fatalf("clinician=%#v err=%v", clinician, err)
	}
	admin, err := presenter.Present(canonical, policy.AudienceAdmin)
	if err != nil || admin.ModelExtra == nil {
		t.Fatalf("admin=%#v err=%v", admin, err)
	}
	if canonical.ModelExtra == nil {
		t.Fatal("presenter mutated canonical content")
	}
}
