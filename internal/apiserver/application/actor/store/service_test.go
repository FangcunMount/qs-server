package store

import (
	"context"
	stderrors "errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/actor/actorctx"
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
	return authz.WithSnapshot(actorctx.WithGrantingUserID(actorctx.WithOperatorOrgID(context.Background(), 7), 9), &authz.Snapshot{ScopeContractVersion: 1, AuthzVersion: 1, Permissions: []authz.Permission{{Resource: "qs:*:*:*", Action: "*", Mode: authz.AuthorizationModeUnconditional, Scopes: []authz.DataScope{{OrgID: 7, Kind: "all_stores"}}}}})
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
	return NewService(r, tx, companyScope{}), r
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

type companyScope struct{}

func (companyScope) ResolveStoreRange(ctx context.Context, org, user int64, resource, action string) (authz.StoreRange, error) {
	snap, _ := authz.FromContext(ctx)
	return snap.ResolveStoreRange(org, resource, action)
}

// A repository with only an embedded nil interface makes any unexpected read fail the test.
func TestAllStoreManagementEntrypointsRejectInvalidCompanyScope(t *testing.T) {
	calls := map[string]func(*Service, context.Context) error{
		"list":   func(s *Service, c context.Context) error { _, e := s.List(c, Actor{7, 9}, port.Filter{}); return e },
		"get":    func(s *Service, c context.Context) error { _, e := s.Get(c, Actor{7, 9}, 2); return e },
		"create": func(s *Service, c context.Context) error { _, e := s.Create(c, Actor{7, 9}, "C", "C", ""); return e },
		"update": func(s *Service, c context.Context) error {
			_, e := s.Update(c, Actor{7, 9}, 2, 1, "B", "", nil)
			return e
		},
		"assign":   func(s *Service, c context.Context) error { _, e := s.Assign(c, Actor{7, 9}, 10, change()); return e },
		"history":  func(s *Service, c context.Context) error { _, e := s.History(c, Actor{7, 9}, 10); return e },
		"progress": func(s *Service, c context.Context) error { _, e := s.Progress(c, Actor{7, 9}); return e },
	}
	for _, kind := range []string{"foreign company", "selected stores", "legacy", "missing resolver", "untrusted actor"} {
		for name, call := range calls {
			t.Run(kind+"/"+name, func(t *testing.T) {
				s, r := fixture(t)
				ctx := adminContext()
				snap, _ := authz.FromContext(ctx)
				switch kind {
				case "foreign company":
					snap.Permissions[0].Scopes[0].OrgID = 8
				case "selected stores":
					snap.Permissions[0].Scopes[0] = authz.DataScope{OrgID: 7, Kind: "stores", StoreIDs: []uint64{2}}
				case "legacy":
					snap.ScopeContractVersion = 0
				case "missing resolver":
					s.scope = nil
				case "untrusted actor":
					ctx = actorctx.WithGrantingUserID(ctx, 99)
				}
				if err := call(s, ctx); err == nil {
					t.Fatal("unauthorized operation accepted")
				}
				if len(r.calls) != 0 || r.history != nil {
					t.Fatal("denied operation reached persistence")
				}
			})
		}
	}
}

type recordingScope struct{ resource, action string }

func (r *recordingScope) ResolveStoreRange(_ context.Context, org, user int64, resource, action string) (authz.StoreRange, error) {
	r.resource, r.action = resource, action
	if org != 7 || user != 9 {
		panic("wrong authenticated actor")
	}
	return authz.StoreRange{}, stderrors.New("stop after recording authorization")
}
func TestStoreManagementChecksExactResourceAndAction(t *testing.T) {
	cases := []struct {
		name, resource, action string
		call                   func(*Service) error
	}{
		{"list", "stores", "list", func(s *Service) error { _, e := s.List(adminContext(), Actor{7, 9}, port.Filter{}); return e }},
		{"get", "stores", "read", func(s *Service) error { _, e := s.Get(adminContext(), Actor{7, 9}, 2); return e }},
		{"create", "stores", "create", func(s *Service) error { _, e := s.Create(adminContext(), Actor{7, 9}, "C", "C", ""); return e }},
		{"update", "stores", "update", func(s *Service) error { _, e := s.Update(adminContext(), Actor{7, 9}, 2, 1, "B", "", nil); return e }},
		{"deactivate", "stores", "update", func(s *Service) error {
			v := false
			_, e := s.Update(adminContext(), Actor{7, 9}, 2, 1, "B", "", &v)
			return e
		}},
		{"assign", "clinicians", "update", func(s *Service) error { _, e := s.Assign(adminContext(), Actor{7, 9}, 10, change()); return e }},
		{"history", "clinicians", "read", func(s *Service) error { _, e := s.History(adminContext(), Actor{7, 9}, 10); return e }},
		{"progress", "clinicians", "list", func(s *Service) error { _, e := s.Progress(adminContext(), Actor{7, 9}); return e }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, repo := fixture(t)
			scope := &recordingScope{}
			s.scope = scope
			if tc.call(s) == nil {
				t.Fatal("authorization error ignored")
			}
			if scope.resource != "qs:actor:collection:"+tc.resource || scope.action != tc.action {
				t.Fatalf("wrong permission: %s %s", scope.resource, scope.action)
			}
			if len(repo.calls) != 0 {
				t.Fatal("authorization failure touched persistence")
			}
		})
	}
}
