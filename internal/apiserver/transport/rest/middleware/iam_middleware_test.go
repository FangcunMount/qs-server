package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	cberrors "github.com/FangcunMount/component-base/pkg/errors"
	operatorapp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/operator"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	pkgmiddleware "github.com/FangcunMount/qs-server/internal/pkg/middleware"
	"github.com/FangcunMount/qs-server/internal/pkg/securityplane"
	"github.com/gin-gonic/gin"
)

func TestUserIdentityMiddlewareProjectsSecurityPrincipalAndScope(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_claims", &pkgmiddleware.UserClaims{
			UserID:       "42",
			AccountID:    "account-1",
			TenantDomain: "fangcun",
			OrgID:        "88",
			SessionID:    "session-1",
			TokenID:      "token-1",
			Roles:        []string{"operator"},
			AMR:          []string{"pwd"},
		})
		c.Next()
	})
	router.Use(UserIdentityMiddleware())
	router.GET("/", func(c *gin.Context) {
		principal, ok := GetPrincipal(c)
		if !ok {
			t.Fatal("expected security principal projection")
		}
		if principal.Kind != securityplane.PrincipalKindUser || principal.Source != securityplane.PrincipalSourceHTTPJWT {
			t.Fatalf("principal kind/source = %#v", principal)
		}
		if principal.UserID != "42" || principal.AccountID != "account-1" || principal.TokenID != "token-1" {
			t.Fatalf("principal = %#v, want projected claims", principal)
		}
		scope, ok := GetOrgScope(c)
		if !ok {
			t.Fatal("expected org scope projection")
		}
		if scope.TenantDomain != "fangcun" || scope.HasOrgID || scope.OrgID != 0 {
			t.Fatalf("scope = %#v, want tenant without JWT org before QS resolver", scope)
		}
		if got := GetUserID(c); got != 42 {
			t.Fatalf("legacy user id = %d, want 42", got)
		}
		c.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
}

func TestResolveOperatorOrgScopeMiddlewareInjectsScopeAndCurrentOperator(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	checker := &operatorScopeCheckerStub{
		resolveResult: &operatorapp.OperatorResult{ID: 7, OrgID: 88, UserID: 42, IsActive: true},
	}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_claims", &pkgmiddleware.UserClaims{
			UserID:       "42",
			TenantDomain: "fangcun",
		})
		c.Next()
	})
	router.Use(UserIdentityMiddleware())
	router.Use(ResolveOperatorOrgScopeMiddleware(checker))
	router.Use(RequireOrgScopeMiddleware())
	router.Use(RequireActiveOperatorMiddleware(checker))
	router.GET("/", func(c *gin.Context) {
		if got := GetOrgID(c); got != 88 {
			t.Fatalf("org_id = %d, want 88", got)
		}
		scope, ok := GetOrgScope(c)
		if !ok || !scope.HasOrgID || scope.OrgID != 88 || scope.TenantDomain != "fangcun" {
			t.Fatalf("scope = %#v, want org 88", scope)
		}
		op := GetCurrentOperator(c)
		if op == nil || op.ID != 7 || op.OrgID != 88 || op.UserID != 42 {
			t.Fatalf("current operator = %+v, want operator 7", op)
		}
		c.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
	if checker.resolveCalls != 1 || checker.requireCalls != 0 {
		t.Fatalf("calls resolve=%d require=%d, want resolve=1 require=0", checker.resolveCalls, checker.requireCalls)
	}
	if checker.lastResolveUserID != 42 || checker.lastResolveOrgID != 0 {
		t.Fatalf("resolve args user=%d org=%d, want user=42 org=0", checker.lastResolveUserID, checker.lastResolveOrgID)
	}
}

func TestResolveOperatorOrgScopeMiddlewarePassesRequestedOrg(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	checker := &operatorScopeCheckerStub{
		resolveResult: &operatorapp.OperatorResult{ID: 7, OrgID: 99, UserID: 42, IsActive: true},
	}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_claims", &pkgmiddleware.UserClaims{
			UserID:       "42",
			TenantDomain: "fangcun",
		})
		c.Next()
	})
	router.Use(UserIdentityMiddleware())
	router.Use(ResolveOperatorOrgScopeMiddleware(checker))
	router.GET("/", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Org-Id", "99")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
	if checker.lastResolveOrgID != 99 {
		t.Fatalf("requested org = %d, want 99", checker.lastResolveOrgID)
	}
}

func TestResolveOperatorOrgScopeMiddlewareRejectsAmbiguousMembership(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	checker := &operatorScopeCheckerStub{
		resolveErr: cberrors.WithCode(code.ErrInvalidArgument, "multiple active organizations; specify X-Org-Id"),
	}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_claims", &pkgmiddleware.UserClaims{
			UserID:       "42",
			TenantDomain: "fangcun",
		})
		c.Next()
	})
	router.Use(UserIdentityMiddleware())
	router.Use(ResolveOperatorOrgScopeMiddleware(checker))
	router.GET("/", func(c *gin.Context) {
		t.Fatal("handler should not run")
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}

type operatorScopeCheckerStub struct {
	resolveResult     *operatorapp.OperatorResult
	resolveErr        error
	requireResult     *operatorapp.OperatorResult
	requireErr        error
	resolveCalls      int
	requireCalls      int
	lastResolveUserID int64
	lastResolveOrgID  int64
}

func (s *operatorScopeCheckerStub) RequireActive(context.Context, int64, int64) (*operatorapp.OperatorResult, error) {
	s.requireCalls++
	return s.requireResult, s.requireErr
}

func (s *operatorScopeCheckerStub) ResolveActive(_ context.Context, userID int64, requestedOrgID int64) (*operatorapp.OperatorResult, error) {
	s.resolveCalls++
	s.lastResolveUserID = userID
	s.lastResolveOrgID = requestedOrgID
	return s.resolveResult, s.resolveErr
}
