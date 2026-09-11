package store

import (
	"context"
	stderrors "errors"
	clinicianDomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/clinician"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/store"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/actorstore"
)

type assignmentRepo struct {
	port.Repository
	target      *domain.Store
	clinician   *clinicianDomain.Clinician
	existing    *port.History
	calls       []string
	history     *port.History
	failHistory bool
}

func (r *assignmentRepo) LockStore(_ context.Context, org int64, id uint64) (*domain.Store, error) {
	r.calls = append(r.calls, "store")
	if r.target.OrgID() != org || r.target.ID() != id {
		return nil, stderrors.New("not found")
	}
	return domain.Restore(r.target.State()), nil
}
func (r *assignmentRepo) LockClinician(_ context.Context, org int64, id uint64) (*clinicianDomain.Clinician, error) {
	r.calls = append(r.calls, "clinician")
	if r.clinician.OrgID() != org || r.clinician.ID().Uint64() != id {
		return nil, stderrors.New("not found")
	}
	v := *r.clinician
	return &v, nil
}
func (r *assignmentRepo) FindChange(context.Context, int64, uint64, string) (*port.History, error) {
	return r.existing, nil
}
func (r *assignmentRepo) InvalidateEntries(context.Context, int64, uint64, int64, time.Time) (int64, error) {
	r.calls = append(r.calls, "invalidate")
	return 3, nil
}
func (r *assignmentRepo) SaveAssignment(_ context.Context, h *port.History) error {
	r.calls = append(r.calls, "save")
	return nil
}
func (r *assignmentRepo) AppendHistory(_ context.Context, h *port.History) error {
	r.calls = append(r.calls, "history")
	if r.failHistory {
		return stderrors.New("history failure")
	}
	r.history = h
	return nil
}
func adminContext() context.Context {
	return authz.WithSnapshot(context.Background(), &authz.Snapshot{Permissions: []authz.Permission{{Resource: "qs:*:*:*", Action: "*", Mode: authz.AuthorizationModeUnconditional}}})
}
func fixture(t *testing.T) (*Service, *assignmentRepo) {
	t.Helper()
	target, err := domain.New(2, 7, "B", "B店", "", 9, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	cl := clinicianDomain.NewClinician(7, "医生", "", "", clinicianDomain.Type("doctor"), "", true)
	cl.SetID(clinicianDomain.NewID(10))
	cl.RestoreStore(nil, 1)
	r := &assignmentRepo{target: target, clinician: cl}
	tx := transaction.RunnerFunc(func(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) })
	return NewService(r, tx), r
}
func change() Change {
	return Change{StoreID: 2, ExpectedVersion: 1, Reason: "配置门店", RequestID: "request-1"}
}
func TestAssignInitialKeepsEntriesAndTransferInvalidates(t *testing.T) {
	for _, transfer := range []bool{false, true} {
		t.Run(map[bool]string{false: "initial", true: "transfer"}[transfer], func(t *testing.T) {
			s, r := fixture(t)
			if transfer {
				v := uint64(1)
				r.clinician.RestoreStore(&v, r.clinician.Version())
			}
			h, err := s.Assign(adminContext(), Actor{7, 9}, 10, change())
			if err != nil {
				t.Fatal(err)
			}
			if h.InvalidatedCount != 0 && !transfer {
				t.Fatal("initial assignment invalidated entries")
			}
			if transfer && (h.InvalidatedCount != 3 || h.Kind != "transfer") {
				t.Fatal("transfer did not invalidate entries")
			}
			if r.calls[0] != "store" || r.calls[1] != "clinician" {
				t.Fatalf("wrong lock order: %v", r.calls)
			}
			if r.history == nil || h.Version != 2 {
				t.Fatal("missing history/version")
			}
		})
	}
}
func TestAssignRejectsMissingPermissionBeforePersistence(t *testing.T) {
	s, r := fixture(t)
	if _, err := s.Assign(context.Background(), Actor{7, 9}, 10, change()); err == nil {
		t.Fatal("missing snapshot accepted")
	}
	if len(r.calls) != 0 {
		t.Fatal("unauthorized request touched persistence")
	}
}
func TestAssignRejectsCompanyMismatchAndStaleVersion(t *testing.T) {
	for _, which := range []string{"company", "version", "inactive"} {
		t.Run(which, func(t *testing.T) {
			s, r := fixture(t)
			a := Actor{7, 9}
			c := change()
			switch which {
			case "company":
				a.OrgID = 8
			case "version":
				c.ExpectedVersion = 2
			case "inactive":
				if err := r.target.SetActive(false, 0, 1, 9, time.Now()); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := s.Assign(adminContext(), a, 10, c); err == nil {
				t.Fatal("invalid assignment accepted")
			}
			if r.history != nil {
				t.Fatal("invalid assignment wrote history")
			}
		})
	}
}
func TestAssignReplayAndSameStoreDoNotWrite(t *testing.T) {
	for _, replay := range []bool{false, true} {
		s, r := fixture(t)
		v := uint64(2)
		r.clinician.RestoreStore(&v, r.clinician.Version())
		r.clinician.RestoreStore(r.clinician.StoreID(), 2)
		if replay {
			r.existing = &port.History{ToStoreID: 2, Reason: change().Reason, ActorID: 9, Version: 2}
		}
		if _, err := s.Assign(adminContext(), Actor{7, 9}, 10, change()); err != nil {
			t.Fatal(err)
		}
		if len(r.calls) != 2 {
			t.Fatalf("replay performed writes: %v", r.calls)
		}
	}
}
func TestAssignRejectsReusedRequestAndPropagatesAuditFailure(t *testing.T) {
	s, r := fixture(t)
	r.existing = &port.History{ToStoreID: 2, Reason: "different", ActorID: 9}
	if _, err := s.Assign(adminContext(), Actor{7, 9}, 10, change()); err == nil {
		t.Fatal("conflicting replay accepted")
	}
	r.existing = nil
	r.failHistory = true
	if h, err := s.Assign(adminContext(), Actor{7, 9}, 10, change()); err == nil || h != nil {
		t.Fatal("audit failure reported as success")
	}
}
