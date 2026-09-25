package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/actor/actorctx"
	operatorapp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/operator"
	authzapp "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	"github.com/gin-gonic/gin"
)

type stubOperatorRoleProjectionUpdater struct {
	lastCtx  context.Context
	lastOp   *operatorapp.OperatorResult
	lastSnap *authzapp.Snapshot
	err      error
	calls    int
}

func (s *stubOperatorRoleProjectionUpdater) PersistFromSnapshot(ctx context.Context, op *operatorapp.OperatorResult, snap *authzapp.Snapshot) error {
	s.calls++
	s.lastCtx = ctx
	s.lastOp = op
	s.lastSnap = snap
	return s.err
}

func (s *stubOperatorRoleProjectionUpdater) PersistFromSnapshotByUser(context.Context, int64, int64, *authzapp.Snapshot) error {
	panic("unexpected call")
}
func (s *stubOperatorRoleProjectionUpdater) SyncRoles(context.Context, int64, uint64) error {
	panic("unexpected call")
}

func TestAuthzSnapshotMiddlewareStoresSnapshotInGinAndRequestContext(t *testing.T) {
	gin.SetMode(gin.TestMode)

	snap := &authzapp.Snapshot{DirectRoles: []string{"qs:admin"}, EffectiveRoles: []string{"qs:admin"}}
	var gotUserID string

	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Set(OrgIDKey, uint64(88))
		c.Set(UserIDStrKey, "701")
		c.Set(UserIDKey, uint64(701))
		c.Next()
	})
	engine.Use(newAuthzSnapshotMiddleware(func(ctx context.Context, userID string) (*authzapp.Snapshot, error) {
		gotUserID = userID
		return snap, nil
	}, nil, nil))
	engine.GET("/check", func(c *gin.Context) {
		if gotUserID != "701" {
			t.Fatalf("loaded user %q", gotUserID)
		}
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
}

func TestAuthzSnapshotMiddlewarePersistsProjectionWhenCurrentOperatorExists(t *testing.T) {
	gin.SetMode(gin.TestMode)

	snap := &authzapp.Snapshot{DirectRoles: []string{"qs:admin"}, EffectiveRoles: []string{"qs:admin"}}
	updater := &stubOperatorRoleProjectionUpdater{}
	operator := &operatorapp.OperatorResult{ID: 801, OrgID: 88, UserID: 701, Name: "Router User", IsActive: true}

	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Set(OrgIDKey, uint64(88))
		c.Set(UserIDStrKey, "701")
		c.Set(UserIDKey, uint64(701))
		c.Set(CurrentOperatorKey, operator)
		c.Next()
	})
	engine.Use(newAuthzSnapshotMiddleware(func(ctx context.Context, userID string) (*authzapp.Snapshot, error) {
		return snap, nil
	}, updater, nil))
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
	operator := &operatorapp.OperatorResult{ID: 801, OrgID: 88, UserID: 701, Name: "Router User", IsActive: true}

	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Set(OrgIDKey, uint64(88))
		c.Set(UserIDStrKey, "701")
		c.Set(UserIDKey, uint64(701))
		c.Set(CurrentOperatorKey, operator)
		c.Next()
	})
	engine.Use(newAuthzSnapshotMiddleware(func(ctx context.Context, userID string) (*authzapp.Snapshot, error) {
		return &authzapp.Snapshot{DirectRoles: []string{"qs:admin"}, EffectiveRoles: []string{"qs:admin"}}, nil
	}, updater, nil))
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

func TestAuthzSnapshotMiddlewareRechecksAfterRoleProjection(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, fail := range []bool{false, true} {
		name := "current proof"
		if fail {
			name = "stale proof"
		}
		t.Run(name, func(t *testing.T) {
			snap := &authzapp.Snapshot{AuthzVersion: 7}
			updater := &stubOperatorRoleProjectionUpdater{}
			operator := &operatorapp.OperatorResult{ID: 801, OrgID: 88, UserID: 701, IsActive: true}
			var verified, handlerCalled bool
			engine := gin.New()
			engine.Use(func(c *gin.Context) {
				c.Set(UserIDStrKey, "701")
				c.Set(UserIDKey, uint64(701))
				c.Set(CurrentOperatorKey, operator)
				c.Next()
			})
			engine.Use(newAuthzSnapshotMiddleware(func(context.Context, string) (*authzapp.Snapshot, error) {
				return snap, nil
			}, updater, func(_ context.Context, got *authzapp.Snapshot) error {
				verified = true
				if updater.calls != 1 || got != snap {
					t.Fatalf("decision check ran before projection or received wrong snapshot")
				}
				if fail {
					return errors.New("committed version advanced")
				}
				return nil
			}))
			engine.GET("/check", func(c *gin.Context) {
				handlerCalled = true
				c.Status(http.StatusNoContent)
			})

			rec := httptest.NewRecorder()
			engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/check", nil))
			if !verified {
				t.Fatal("decision-time proof was not checked")
			}
			if fail {
				if rec.Code != http.StatusServiceUnavailable || handlerCalled {
					t.Fatalf("stale proof admitted request: status=%d handler=%v", rec.Code, handlerCalled)
				}
				return
			}
			if rec.Code != http.StatusNoContent || !handlerCalled {
				t.Fatalf("current proof rejected request: status=%d handler=%v", rec.Code, handlerCalled)
			}
		})
	}
}
