package assessmententry

import (
	"context"
	"errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/actor/actorctx"
	authz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	authztest "github.com/FangcunMount/qs-server/internal/apiserver/application/authz/testutil"
	transaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	"testing"
)

type entryManagementScope struct {
	all    bool
	action string
}

func (s entryManagementScope) ResolveStoreRange(_ context.Context, org, user int64, resource, action string) (authz.StoreRange, error) {
	if org != 1 || user != 9 || resource != "qs:actor:collection:clinicians" || action != s.action {
		panic("wrong entry administration authority")
	}
	return authz.StoreRange{AllStores: s.all, StoreIDs: []uint64{7}}, nil
}
func TestEntryManagementDeniesSelectedStoreBeforeReadOrWrite(t *testing.T) {
	ctx := authztest.WithPermission(actorctx.WithGrantingUserID(actorctx.WithOperatorOrgID(context.Background(), 1), 9), "qs:*:*:*", "*")
	for _, method := range []string{"create", "read", "list", "activate", "deactivate"} {
		action := "update"
		if method == "read" || method == "list" {
			action = method
		}
		s := &service{managed: true, managementScope: entryManagementScope{action: action}}
		var err error
		switch method {
		case "create":
			_, err = s.Create(ctx, CreateAssessmentEntryDTO{OrgID: 1})
		case "read":
			_, err = s.GetByID(ctx, 10)
		case "list":
			_, err = s.ListByClinician(ctx, ListAssessmentEntryDTO{OrgID: 1})
		case "activate":
			_, err = s.Reactivate(ctx, 10)
		case "deactivate":
			_, err = s.Deactivate(ctx, 10)
		}
		if err == nil {
			t.Fatal("selected-store administration permitted")
		}
		s.managementScope = entryManagementScope{all: true, action: action}
		if err := s.authorizeManagement(ctx, 1, action); err != nil {
			t.Fatal(err)
		}
		if err := s.authorizeManagement(ctx, 2, action); err == nil {
			t.Fatal("foreign company permitted")
		}
	}
}
func TestManagedEntryPublicResolveRetainsIndependentTransactionEntry(t *testing.T) {
	sentinel := errors.New("public resolution transaction")
	called := false
	s := &service{managed: true, uow: transaction.RunnerFunc(func(context.Context, func(context.Context) error) error { called = true; return sentinel })}
	_, err := s.Resolve(context.Background(), "public-token")
	if !called || !errors.Is(err, sentinel) {
		t.Fatalf("public resolve incorrectly requires operator scope: %v", err)
	}
}
