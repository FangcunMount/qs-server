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
