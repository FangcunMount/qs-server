package testeestore

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/actor/actorctx"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/store"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/testeestore"
)

type repository struct {
	port.Repository
	target          *store.Store
	subject         *testee.Testee
	previous, saved *port.History
	calls           []string
	failHistory     bool
}

func (r *repository) LockStore(_ context.Context, org int64, id uint64) (*store.Store, error) {
	r.calls = append(r.calls, "store")
	if org != r.target.OrgID() || id != r.target.ID() {
		return nil, errors.New("not found")
	}
	return r.target, nil
}
func (r *repository) LockTestee(_ context.Context, org int64, id uint64) (*testee.Testee, error) {
	r.calls = append(r.calls, "testee")
	if org != r.subject.OrgID() || id != r.subject.ID().Uint64() {
		return nil, errors.New("not found")
	}
	copy := *r.subject
	return &copy, nil
}
func (r *repository) FindChange(context.Context, int64, uint64, string) (*port.History, error) {
	return r.previous, nil
}
func (r *repository) SaveOwnership(_ context.Context, h *port.History, _ uint32) error {
	r.calls = append(r.calls, "save")
	r.saved = h
	return nil
}
func (r *repository) AppendHistory(context.Context, *port.History) error {
	r.calls = append(r.calls, "history")
	if r.failHistory {
		return errors.New("history failure")
	}
	return nil
}
func admin() context.Context {
	return authz.WithSnapshot(actorctx.WithGrantingUserID(actorctx.WithOperatorOrgID(context.Background(), 7), 9), &authz.Snapshot{ScopeContractVersion: 1, AuthzVersion: 1, Permissions: []authz.Permission{{Resource: "qs:*:*:*", Action: "*", Mode: authz.AuthorizationModeUnconditional, Scopes: []authz.DataScope{{OrgID: 7, Kind: "all_stores"}}}}})
}
func fixture(t *testing.T) (*Service, *repository) {
	t.Helper()
	target, err := store.New(2, 7, "B", "B店", "", 9, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	subject := testee.NewTestee(7, "测试", testee.Gender(0), nil)
	subject.SetID(10)
	r := &repository{target: target, subject: subject}
	tx := transaction.RunnerFunc(func(ctx context.Context, fn func(context.Context) error) error {
		before := r.saved
		err := fn(ctx)
		if err != nil {
			r.saved = before
		}
		return err
	})
	return NewService(r, tx, scopeChecker{}), r
}
func command() Change {
	return Change{StoreID: 2, ExpectedVersion: 1, Reason: "归属配置", RequestID: "req-1"}
}
func TestInitialAndTransferUseOneOrderedTransaction(t *testing.T) {
	for _, transfer := range []bool{false, true} {
		s, r := fixture(t)
		kind := "initial"
		if transfer {
			id := uint64(1)
			r.subject.RestoreStore(&id, 1)
			kind = "transfer"
		}
		var h *port.History
		var err error
		if transfer {
			h, err = s.Transfer(admin(), Actor{7, 9}, 10, command())
		} else {
			h, err = s.AssignInitial(admin(), Actor{7, 9}, 10, command())
		}
		if err != nil {
			t.Fatal(err)
		}
		if h.Kind != kind || h.Version != 2 || h.ActorID != 9 || h.RequestID != "req-1" {
			t.Fatalf("invalid audit: %+v", h)
		}
		if !reflect.DeepEqual(r.calls, []string{"store", "testee", "save", "history"}) {
			t.Fatalf("order: %v", r.calls)
		}
	}
}
func TestDeniedAndForeignRequestsCannotWrite(t *testing.T) {
	for _, tc := range []struct {
		ctx   context.Context
		actor Actor
	}{{context.Background(), Actor{7, 9}}, {admin(), Actor{8, 9}}, {admin(), Actor{7, 0}}} {
		s, r := fixture(t)
		if _, err := s.AssignInitial(tc.ctx, tc.actor, 10, command()); err == nil {
			t.Fatal("invalid actor accepted")
		}
		if r.saved != nil {
			t.Fatal("unauthorized write")
		}
	}
}
func TestRepeatedRequestReturnsHistoricalResultWithoutReapplying(t *testing.T) {
	s, r := fixture(t)
	first, err := s.AssignInitial(admin(), Actor{7, 9}, 10, command())
	if err != nil {
		t.Fatal(err)
	}
	r.previous = first
	r.calls = nil
	id := uint64(3)
	r.subject.RestoreStore(&id, 3) // a subsequent audited transfer must not be undone
	again, err := s.AssignInitial(admin(), Actor{7, 9}, 10, command())
	if err != nil || again.ID != first.ID {
		t.Fatalf("replay: %v", err)
	}
	if !reflect.DeepEqual(r.calls, []string{"store", "testee"}) || *r.subject.StoreID() != 3 {
		t.Fatal("replay rewrote current ownership")
	}
	c := command()
	c.Reason = "different"
	if _, err = s.AssignInitial(admin(), Actor{7, 9}, 10, c); err == nil {
		t.Fatal("conflicting request reused")
	}
	if _, err = s.Transfer(admin(), Actor{7, 9}, 10, command()); err == nil {
		t.Fatal("initial request reused for transfer")
	}
}
func TestFailedAuditReturnsNoSuccessAndRollsBackWrite(t *testing.T) {
	s, r := fixture(t)
	r.failHistory = true
	h, err := s.AssignInitial(admin(), Actor{7, 9}, 10, command())
	if err == nil || h != nil || r.saved != nil {
		t.Fatal("failed audit left committed result")
	}
}
func TestAlreadyOwnedTesteeNeedsExplicitTransfer(t *testing.T) {
	s, r := fixture(t)
	id := uint64(1)
	r.subject.RestoreStore(&id, 1)
	if _, err := s.AssignInitial(admin(), Actor{7, 9}, 10, command()); err == nil {
		t.Fatal("implicit transfer accepted")
	}
	if r.saved != nil {
		t.Fatal("implicit transfer wrote ownership")
	}
}

// The production resolver additionally verifies active Operator membership.
type scopeChecker struct{}

func (scopeChecker) ResolveStoreRange(ctx context.Context, org, user int64, resource, action string) (authz.StoreRange, error) {
	snapshot, _ := authz.FromContext(ctx)
	return snapshot.ResolveStoreRange(org, resource, action)
}
func TestHeadquartersOwnershipRequiresCurrentCompanyActionScope(t *testing.T) {
	for _, tc := range []struct {
		name, action, kind string
		org                int64
	}{
		{"foreign company", "*", "all_stores", 8},
		{"selected store", "*", "stores", 7},
		{"read cannot transfer", "read", "all_stores", 7},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, r := fixture(t)
			snap, _ := authz.FromContext(admin())
			// Keep administrative capability separate from the exact scoped action.
			snap.Permissions[0].Scopes = nil
			snap.Permissions = append(snap.Permissions, authz.Permission{Resource: "qs:actor:collection:testees", Action: tc.action, Mode: authz.AuthorizationModeUnconditional, Scopes: []authz.DataScope{{OrgID: tc.org, Kind: tc.kind}}})
			if tc.kind == "stores" {
				snap.Permissions[1].Scopes[0].StoreIDs = []uint64{2}
			}
			ctx := authz.WithSnapshot(admin(), snap)
			if _, err := s.Transfer(ctx, Actor{7, 9}, 10, command()); err == nil {
				t.Fatal("out-of-scope transfer accepted")
			}
			if len(r.calls) != 0 {
				t.Fatal("denied request reached repository")
			}
		})
	}
}

func (r *repository) History(context.Context, int64, uint64, uint64, int) ([]port.History, error) {
	r.calls = append(r.calls, "read history")
	return []port.History{}, nil
}
func TestOwnershipHistoryRequiresReadScope(t *testing.T) {
	for _, action := range []string{"read", "update"} {
		t.Run(action, func(t *testing.T) {
			s, r := fixture(t)
			snap, _ := authz.FromContext(admin())
			snap.Permissions[0].Scopes = nil
			snap.Permissions = append(snap.Permissions, authz.Permission{Resource: "qs:actor:collection:testees", Action: action, Mode: authz.AuthorizationModeUnconditional, Scopes: []authz.DataScope{{OrgID: 7, Kind: "all_stores"}}})
			result, err := s.History(authz.WithSnapshot(admin(), snap), Actor{7, 9}, 10, 0, 20)
			if action == "read" {
				if err != nil || len(r.calls) != 1 {
					t.Fatalf("authorized history: %v %v", err, r.calls)
				}
			} else if err == nil || result != nil || len(r.calls) != 0 {
				t.Fatal("update permission exposed history")
			}
		})
	}
}

type unavailableOperator struct{}

func (unavailableOperator) ResolveStoreRange(context.Context, int64, int64, string, string) (authz.StoreRange, error) {
	return authz.StoreRange{}, errors.New("operator inactive")
}
func TestOwnershipRejectsMissingOrInactiveScopeResolver(t *testing.T) {
	for _, scope := range []CompanyScope{nil, unavailableOperator{}} {
		s, r := fixture(t)
		s.scope = scope
		if _, err := s.AssignInitial(admin(), Actor{7, 9}, 10, command()); err == nil || len(r.calls) != 0 {
			t.Fatal("missing active operator reached mutation")
		}
		if _, err := s.History(admin(), Actor{7, 9}, 10, 0, 20); err == nil || len(r.calls) != 0 {
			t.Fatal("missing active operator reached history")
		}
	}
}
