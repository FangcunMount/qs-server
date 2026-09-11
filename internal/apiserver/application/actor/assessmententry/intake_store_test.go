package assessmententry

import (
	"context"
	"errors"
	"testing"
	"time"

	entry "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/assessmententry"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/clinician"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/store"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/testeestore"
)

type intakeOwnershipRepo struct {
	port.IntakeRepository
	subject *testee.Testee
	saved   *port.History
	history *port.History
	fail    bool
}

func (r *intakeOwnershipRepo) LockTestee(context.Context, int64, uint64) (*testee.Testee, error) {
	v := *r.subject
	return &v, nil
}
func (r *intakeOwnershipRepo) SaveOwnership(_ context.Context, h *port.History, _ uint32) error {
	r.saved = h
	return nil
}
func (r *intakeOwnershipRepo) AppendHistory(_ context.Context, h *port.History) error {
	if r.fail {
		return errors.New("audit failed")
	}
	r.history = h
	return nil
}
func intakeOwnershipFixture(t *testing.T) (*intakeUseCase, *intakeState, *store.Store, *intakeOwnershipRepo) {
	t.Helper()
	target, err := store.New(2, 7, "B", "B店", "", 9, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	subject := testee.NewTestee(7, "test", testee.Gender(0), nil)
	subject.SetID(10)
	doctor := clinician.NewClinician(7, "doctor", "", "", clinician.Type("doctor"), "", true)
	doctor.SetID(20)
	e := entry.NewAssessmentEntry(7, doctor.ID(), "token", entry.TargetTypeScale, "scale", "v1", true, nil)
	e.SetID(30)
	state := &intakeState{entry: e, clinician: doctor, testee: subject, intakeAt: time.Now()}
	repo := &intakeOwnershipRepo{subject: subject}
	return newIntakeUseCase(&service{ownership: repo}), state, target, repo
}
func TestIntakeInitialOwnershipRecordsAnonymousSource(t *testing.T) {
	u, state, target, r := intakeOwnershipFixture(t)
	if err := u.assignIntakeStore(context.Background(), state, target, 99); err != nil {
		t.Fatal(err)
	}
	h := r.history
	if h == nil || h.ActorID != 0 || h.Kind != "scan_initial" || h.EntryID == nil || *h.EntryID != 30 || h.ClinicianID == nil || *h.ClinicianID != 20 {
		t.Fatalf("missing scan audit: %+v", h)
	}
	if *state.testee.StoreID() != 2 || state.testee.StoreVersion() != 2 {
		t.Fatal("first ownership not returned")
	}
}
func TestIntakePreservesExistingStoreAcrossDifferentDoctorScans(t *testing.T) {
	u, state, target, r := intakeOwnershipFixture(t)
	id := uint64(1)
	r.subject.RestoreStore(&id, 3)
	if err := u.assignIntakeStore(context.Background(), state, target, 99); err != nil {
		t.Fatal(err)
	}
	if r.saved != nil || r.history != nil || *state.testee.StoreID() != 1 || state.testee.StoreVersion() != 3 {
		t.Fatal("scan changed existing ownership")
	}
}
func TestIntakeOwnershipAuditFailureIsPropagatedToOuterTransaction(t *testing.T) {
	u, state, target, r := intakeOwnershipFixture(t)
	r.fail = true
	if err := u.assignIntakeStore(context.Background(), state, target, 99); err == nil {
		t.Fatal("audit failure swallowed")
	}
	if state.testee.StoreID() != nil {
		t.Fatal("failed ownership exposed as successful state")
	}
}
