package answersheet

import (
	"context"
	"errors"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	domainanswersheet "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	rulesetport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

type admissionBindingStub struct {
	binding rulesetport.AssessmentBinding
	ok      bool
	err     error
}

func (s admissionBindingStub) ResolveByQuestionnaire(context.Context, string, string) (rulesetport.Ref, bool, error) {
	return s.binding.Ref, s.ok, s.err
}

func (s admissionBindingStub) ResolveAssessmentBinding(context.Context, string, string) (rulesetport.AssessmentBinding, bool, error) {
	return s.binding, s.ok, s.err
}

func TestResolveAdmissionFreezesAssessmentRelease(t *testing.T) {
	t.Parallel()
	svc := &submissionService{binding: admissionBindingStub{
		binding: rulesetport.AssessmentBinding{Ref: rulesetport.Ref{
			Kind: modelcatalog.KindScale, Code: "MODEL-OLD", Version: "1.0.0", Title: "old",
		}},
		ok: true,
	}}
	got, err := svc.resolveAdmission(context.Background(), "Q-1", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if !got.RequiresAssessment() || got.ModelCode() != "MODEL-OLD" || got.ModelVersion() != "1.0.0" {
		t.Fatalf("admission = %#v", got)
	}
}

func TestResolveAdmissionIndependentWhenUnbound(t *testing.T) {
	t.Parallel()
	svc := &submissionService{binding: admissionBindingStub{ok: false}}
	got, err := svc.resolveAdmission(context.Background(), "Q-IND", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if got.RequiresAssessment() || got.Purpose() != domainanswersheet.AdmissionPurposeIndependentQuestionnaire {
		t.Fatalf("admission = %#v", got)
	}
}

func TestResolveAdmissionFailsClosedOnBindingError(t *testing.T) {
	t.Parallel()
	svc := &submissionService{binding: admissionBindingStub{err: errors.New("mongo timeout")}}
	if _, err := svc.resolveAdmission(context.Background(), "Q-1", "1.0.0"); err == nil {
		t.Fatal("expected binding dependency error")
	}
}

func TestAdmissionEventRoundTrip(t *testing.T) {
	t.Parallel()
	a, err := domainanswersheet.NewAssessmentAdmission("Q", "1", "scale", "", "", "M", "2.0.0", "title")
	if err != nil {
		t.Fatal(err)
	}
	payload := a.ToEventPayload()
	back, err := domainanswersheet.AdmissionFromEventPayload(payload)
	if err != nil {
		t.Fatal(err)
	}
	if back.ModelCode() != "M" || back.ModelVersion() != "2.0.0" || !back.RequiresAssessment() {
		t.Fatalf("round trip = %#v", back)
	}
}
