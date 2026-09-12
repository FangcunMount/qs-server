package answeringstart

import (
	"context"
	"errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answeringstart"
	sheet "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/answeringstart"
	"testing"
	"time"
)

type memory struct {
	record         *domain.Record
	subject        *testee.Testee
	inserts, locks int
	failure        error
	denied         error
}

func (m *memory) FindRequest(_ context.Context, org int64, user uint64, key string) (*domain.Record, error) {
	if m.record != nil {
		i := m.record.Intent()
		if i.OrgID == org && i.UserID == user && i.RequestKey == key {
			return m.record, nil
		}
	}
	return nil, nil
}
func (m *memory) Find(_ context.Context, id uint64) (*domain.Record, error) {
	if m.record != nil && m.record.Context().ID() == id {
		return m.record, nil
	}
	return nil, nil
}
func (m *memory) Insert(_ context.Context, r *domain.Record) error {
	if m.failure != nil {
		return m.failure
	}
	m.inserts++
	m.record = r
	return nil
}
func (m *memory) LockTestee(context.Context, int64, uint64) (*testee.Testee, error) {
	m.locks++
	return m.subject, nil
}
func (m *memory) ValidateStart(context.Context, *domain.Intent) error { return m.denied }
func fixture() (*Service, *memory, domain.Intent) {
	owner := testee.NewTestee(1, "test", 0, nil)
	store := uint64(10)
	owner.RestoreStore(&store, 2)
	m := &memory{subject: owner}
	tx := transaction.RunnerFunc(func(ctx context.Context, f func(context.Context) error) error { return f(ctx) })
	s := NewService(m, tx, m, m)
	s.newID = func() uint64 { return 99 }
	s.now = func() time.Time { return time.Date(2026, 9, 30, 23, 0, 0, 0, time.UTC) }
	return s, m, domain.Intent{OrgID: 1, UserID: 2, TesteeID: 3, RequestKey: "start-one", QuestionnaireCode: "Q", QuestionnaireVersion: "1", Origin: sheet.OriginRef{Type: sheet.OriginTypeSelfService}}
}
func TestReplayAndSubmissionKeepOriginalStoreAfterTransfer(t *testing.T) {
	s, m, i := fixture()
	first, err := s.Start(context.Background(), i)
	if err != nil || !first.Created {
		t.Fatalf("first: %+v %v", first, err)
	}
	next := uint64(20)
	m.subject.RestoreStore(&next, 3)
	replay, err := s.Start(context.Background(), i)
	if err != nil || replay.Created || *replay.Record.Context().StoreID() != 10 || m.inserts != 1 || m.locks != 1 {
		t.Fatalf("replay: %+v %v", replay, err)
	}
	c, err := s.ResolveSubmission(context.Background(), 99, i)
	if err != nil || *c.StoreID() != 10 {
		t.Fatalf("submission: %+v %v", c, err)
	}
	i.RequestKey = "new-round"
	s.newID = func() uint64 { return 100 }
	fresh, err := s.Start(context.Background(), i)
	if err != nil || *fresh.Record.Context().StoreID() != 20 {
		t.Fatalf("new round: %+v %v", fresh, err)
	}
}
func TestStartRejectsReplayMismatchAndAdmissionLoss(t *testing.T) {
	s, m, i := fixture()
	_, _ = s.Start(context.Background(), i)
	other := i
	other.TesteeID = 4
	if _, err := s.Start(context.Background(), other); !errors.Is(err, port.ErrConflict) {
		t.Fatalf("mismatch: %v", err)
	}
	m.denied = errors.New("relationship revoked")
	if _, err := s.Start(context.Background(), i); !errors.Is(err, m.denied) {
		t.Fatalf("admission: %v", err)
	}
	if m.inserts != 1 {
		t.Fatal("rejected request wrote data")
	}
}
func TestStartInsertFailureReturnsNoAcceptedRecord(t *testing.T) {
	s, m, i := fixture()
	m.failure = errors.New("transaction failed")
	result, err := s.Start(context.Background(), i)
	if !errors.Is(err, m.failure) || result.Record != nil || m.record != nil {
		t.Fatalf("result: %+v %v", result, err)
	}
}
func TestSubmissionRejectsInvalidReferenceInsteadOfUsingCurrentStore(t *testing.T) {
	s, _, i := fixture()
	_, _ = s.Start(context.Background(), i)
	for _, change := range []func(*domain.Intent){func(v *domain.Intent) { v.UserID++ }, func(v *domain.Intent) { v.OrgID++ }, func(v *domain.Intent) { v.TesteeID++ }, func(v *domain.Intent) { v.QuestionnaireVersion = "2" }, func(v *domain.Intent) { v.Origin = sheet.OriginRef{Type: sheet.OriginTypePlanTask, ID: "task"} }} {
		other := i
		change(&other)
		if _, err := s.ResolveSubmission(context.Background(), 99, other); err == nil {
			t.Fatal("mismatched reference accepted")
		}
	}
	if _, err := s.ResolveSubmission(context.Background(), 123, i); err == nil {
		t.Fatal("missing reference accepted")
	}
	c, err := s.ResolveSubmission(context.Background(), 0, i)
	if err != nil || c.State() != sheet.StartLegacy {
		t.Fatal("legacy compatibility broken")
	}
}
