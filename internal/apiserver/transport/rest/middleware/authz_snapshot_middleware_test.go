package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/actor/actorctx"
	authzapp "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	domainoperator "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/operator"
	"github.com/gin-gonic/gin"
)

type stubOperatorRoleProjectionUpdater struct {
	lastCtx  context.Context
	lastOp   *domainoperator.Operator
	lastSnap *authzapp.Snapshot
	err      error
	calls    int
}

func (s *stubOperatorRoleProjectionUpdater) PersistFromSnapshot(ctx context.Context, op *domainoperator.Operator, snap *authzapp.Snapshot) error {
	s.calls++
	s.lastCtx = ctx
	s.lastOp = op
	s.lastSnap = snap
	return s.err
}

func TestAuthzSnapshotMiddlewareStoresSnapshotInGinAndRequestContext(t *testing.T) {
	gin.SetMode(gin.TestMode)

	snap := &authzapp.Snapshot{Roles: []string{"qs:admin"}}
	var gotTenantID string
	var gotUserID string

	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Set(TenantIDKey, "88")
		c.Set(UserIDStrKey, "701")
		c.Set(UserIDKey, uint64(701))
		c.Next()
	})
	engine.Use(newAuthzSnapshotMiddleware(func(ctx context.Context, tenantID, userID string) (*authzapp.Snapshot, error) {
		gotTenantID = tenantID
		gotUserID = userID
		return snap, nil
	}, nil))
	engine.GET("/check", func(c *gin.Context) {
		if got := GetAuthzSnapshot(c); got != snap {
			t.Fatalf("snapshot in gin context = %#v, want %#v", got, snap)
		}
		fromCtx, ok := authzapp.FromContext(c.Request.Context())
		if !ok || fromCtx != snap {
			t.Fatalf("snapshot in request context = %#v, want %#v", fromCtx, snap)
		}
		if got := actorctx.GrantingUserID(c.Request.Context()); got != 701 {
			t.Fatalf("granting user id = %d, want 701", got)
		}
		c.Status(http.StatusNoContent)
	})

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/check", nil))

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if gotTenantID != "88" || gotUserID != "701" {
		t.Fatalf("load called with tenant=%q user=%q, want tenant=88 user=701", gotTenantID, gotUserID)
	}
}

func TestAuthzSnapshotMiddlewarePersistsProjectionWhenCurrentOperatorExists(t *testing.T) {
	gin.SetMode(gin.TestMode)

	snap := &authzapp.Snapshot{Roles: []string{"qs:admin"}}
	updater := &stubOperatorRoleProjectionUpdater{}
	operator := domainoperator.NewOperator(88, 701, "Router User")

	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Set(TenantIDKey, "88")
		c.Set(UserIDStrKey, "701")
		c.Set(UserIDKey, uint64(701))
		c.Set(CurrentOperatorKey, operator)
		c.Next()
	})
	engine.Use(newAuthzSnapshotMiddleware(func(ctx context.Context, tenantID, userID string) (*authzapp.Snapshot, error) {
		return snap, nil
	}, updater))
	engine.GET("/check", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/check", nil))

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if updater.calls != 1 {
		t.Fatalf("updater calls = %d, want 1", updater.calls)
	}
	if updater.lastOp != operator || updater.lastSnap != snap {
		t.Fatalf("unexpected updater args: op=%#v snap=%#v", updater.lastOp, updater.lastSnap)
	}
	if got := actorctx.GrantingUserID(updater.lastCtx); got != 701 {
		t.Fatalf("granting user id in updater ctx = %d, want 701", got)
	}
}

func TestAuthzSnapshotMiddlewareUpdaterFailureDoesNotAbortRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	updater := &stubOperatorRoleProjectionUpdater{err: errors.New("boom")}
	operator := domainoperator.NewOperator(88, 701, "Router User")

	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Set(TenantIDKey, "88")
		c.Set(UserIDStrKey, "701")
		c.Set(UserIDKey, uint64(701))
		c.Set(CurrentOperatorKey, operator)
		c.Next()
	})
	engine.Use(newAuthzSnapshotMiddleware(func(ctx context.Context, tenantID, userID string) (*authzapp.Snapshot, error) {
		return &authzapp.Snapshot{Roles: []string{"qs:admin"}}, nil
	}, updater))
	engine.GET("/check", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/check", nil))

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if updater.calls != 1 {
		t.Fatalf("updater calls = %d, want 1", updater.calls)
	}
}
