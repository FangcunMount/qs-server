package clinician

import (
	"context"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/actor/actorctx"
	authz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	authztest "github.com/FangcunMount/qs-server/internal/apiserver/application/authz/testutil"
	"testing"
)

type clinicianHQScope struct {
	all    bool
	action string
}

func (s clinicianHQScope) ResolveStoreRange(_ context.Context, org, user int64, resource, action string) (authz.StoreRange, error) {
	if org != 1 || user != 9 || resource != "qs:actor:collection:clinicians" || action != s.action {
		panic("wrong headquarters authority")
	}
	return authz.StoreRange{AllStores: s.all, StoreIDs: []uint64{7}}, nil
}
func TestClinicianLifecycleDeniesWithoutHeadquartersBeforeDependencies(t *testing.T) {
	ctx := authztest.WithPermission(actorctx.WithGrantingUserID(actorctx.WithOperatorOrgID(context.Background(), 1), 9), "qs:*:*:*", "*")
	for _, action := range []string{"create", "update", "delete"} {
		svc := NewOperatorLifecycleService(nil, nil, nil, clinicianHQScope{action: action})
		var err error
		switch action {
		case "create":
			_, err = svc.Register(ctx, RegisterClinicianDTO{OrgID: 1})
		case "update":
			_, err = svc.Activate(ctx, 10)
		case "delete":
			err = svc.Delete(ctx, 10)
		}
		if err == nil {
			t.Fatal("selected-store authority allowed headquarters mutation")
		}
		if err := authorizeHeadquarters(ctx, clinicianHQScope{all: true, action: action}, 1, action); err != nil {
			t.Fatal(err)
		}
		if err := authorizeHeadquarters(ctx, clinicianHQScope{all: true, action: action}, 2, action); err == nil {
			t.Fatal("foreign company authorized")
		}
		if err := authorizeHeadquarters(context.Background(), nil, 1, action); err == nil {
			t.Fatal("missing actor authorized")
		}
	}
}
