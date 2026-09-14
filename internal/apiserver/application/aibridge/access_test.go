package aibridge

import (
	"context"
	"errors"
	evaluationtestee "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/actorreadmodel"
	"testing"
)

type currentAccessFixture struct {
	row           *actorreadmodel.TesteeRow
	allowed       bool
	linkErr       error
	calls         int
	assessmentErr error
}

func (f *currentAccessFixture) GetTestee(context.Context, uint64) (*actorreadmodel.TesteeRow, error) {
	return f.row, nil
}
func (f *currentAccessFixture) HasActiveProfileLink(_ context.Context, user, profile string) (bool, error) {
	if user != "parent" || profile != "9" {
		panic("incorrect identity")
	}
	return f.allowed, f.linkErr
}
func (f *currentAccessFixture) AuthorizeAssessment(_ context.Context, actor evaluationtestee.Actor, id uint64) error {
	if actor.TesteeID != 7 || id != 42 {
		panic("incorrect assessment")
	}
	f.calls++
	return f.assessmentErr
}
func TestCurrentAccessRechecksRelationshipEveryTime(t *testing.T) {
	profile := uint64(9)
	f := &currentAccessFixture{row: &actorreadmodel.TesteeRow{ID: 7, OrgID: 1, ProfileID: &profile}, allowed: true}
	a := &CurrentAccess{Testees: f, Links: f, Assessments: f}
	if err := a.Authorize(context.Background(), Actor{"1", "parent"}, "7", []string{"42"}); err != nil {
		t.Fatal(err)
	}
	f.allowed = false
	if err := a.Authorize(context.Background(), Actor{"1", "parent"}, "7", []string{"42"}); !errors.Is(err, ErrAccessDenied) {
		t.Fatal(err)
	}
	if f.calls != 1 {
		t.Fatal("revoked caller reached assessment")
	}
}
func TestCurrentAccessFailsClosed(t *testing.T) {
	for _, name := range []string{"wrong_org", "missing_profile", "iam_down", "assessment_denied", "duplicate", "unconfigured"} {
		t.Run(name, func(t *testing.T) {
			profile := uint64(9)
			f := &currentAccessFixture{row: &actorreadmodel.TesteeRow{ID: 7, OrgID: 1, ProfileID: &profile}, allowed: true}
			a := &CurrentAccess{Testees: f, Links: f, Assessments: f}
			ids := []string{"42"}
			want := ErrAccessDenied
			switch name {
			case "wrong_org":
				f.row.OrgID = 2
			case "missing_profile":
				f.row.ProfileID = nil
			case "iam_down":
				f.linkErr = errors.New("offline")
				want = ErrAccessUnavailable
			case "assessment_denied":
				f.assessmentErr = ErrAccessDenied
			case "duplicate":
				ids = append(ids, "42")
				want = ErrInvalid
			case "unconfigured":
				a.Links = nil
				want = ErrAccessUnavailable
			}
			if err := a.Authorize(context.Background(), Actor{"1", "parent"}, "7", ids); !errors.Is(err, want) {
				t.Fatalf("got %v want %v", err, want)
			}
			if name != "assessment_denied" && f.calls != 0 {
				t.Fatal("denied identity reached assessment")
			}
		})
	}
}

func TestParticipantActorResolvesOrganizationFromAuthorizedTestee(t *testing.T) {
	for _, org := range []uint64{0, 1} {
		profile := uint64(9)
		f := &currentAccessFixture{row: &actorreadmodel.TesteeRow{ID: 7, OrgID: 1, ProfileID: &profile}, allowed: true}
		a := &CurrentAccess{Testees: f, Links: f, Assessments: f}
		actor, err := a.ResolveParticipantActor(context.Background(), "parent", org, 7, 42)
		if err != nil || actor != (Actor{"1", "parent"}) || f.calls != 1 {
			t.Fatalf("org=%d actor=%v err=%v calls=%d", org, actor, err, f.calls)
		}
		f.allowed = false
		if _, err := a.ResolveParticipantActor(context.Background(), "parent", org, 7, 42); !errors.Is(err, ErrAccessDenied) {
			t.Fatalf("revoked relationship accepted: %v", err)
		}
	}
}

func TestParticipantActorCannotInventOrganizationOrBypassAccess(t *testing.T) {
	for _, name := range []string{"different_claim", "missing_testee", "wrong_testee", "missing_org", "missing_profile", "iam_down", "assessment_denied", "unconfigured", "missing_subject"} {
		t.Run(name, func(t *testing.T) {
			profile := uint64(9)
			f := &currentAccessFixture{row: &actorreadmodel.TesteeRow{ID: 7, OrgID: 1, ProfileID: &profile}, allowed: true}
			a := &CurrentAccess{Testees: f, Links: f, Assessments: f}
			org, subject := uint64(0), "parent"
			want := ErrAccessDenied
			switch name {
			case "different_claim":
				org = 2
			case "missing_testee":
				f.row = nil
			case "wrong_testee":
				f.row.ID = 8
			case "missing_org":
				f.row.OrgID = 0
			case "missing_profile":
				f.row.ProfileID = nil
			case "iam_down":
				f.linkErr = errors.New("offline")
				want = ErrAccessUnavailable
			case "assessment_denied":
				f.assessmentErr = ErrAccessDenied
			case "unconfigured":
				a = nil
				want = ErrAccessUnavailable
			case "missing_subject":
				subject = ""
				want = ErrInvalid
			}
			actor, err := a.ResolveParticipantActor(context.Background(), subject, org, 7, 42)
			if !errors.Is(err, want) || actor != (Actor{}) {
				t.Fatalf("actor=%v error=%v want=%v", actor, err, want)
			}
		})
	}
	// An execution callback must still provide the organization fixed at admission.
	if err := (&CurrentAccess{}).Authorize(context.Background(), Actor{"0", "parent"}, "7", []string{"42"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("missing callback organization accepted: %v", err)
	}
}
