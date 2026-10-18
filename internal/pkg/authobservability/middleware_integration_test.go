package authobservability_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	sdk "github.com/FangcunMount/iam/v5/pkg/sdk"
	authjwks "github.com/FangcunMount/iam/v5/pkg/sdk/auth/jwks"
	auth "github.com/FangcunMount/iam/v5/pkg/sdk/auth/verifier"
	qsgrpc "github.com/FangcunMount/qs-server/internal/pkg/grpc"
	"github.com/FangcunMount/qs-server/internal/pkg/middleware"
	"github.com/gin-gonic/gin"
	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jws"
	"github.com/lestrrat-go/jwx/v2/jwt"
	"github.com/prometheus/client_golang/prometheus"
	ggrpc "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestVerifiedRESTClaimsObserveOnlySelectedSources(t *testing.T) {
	privateKey, manager := newContractJWKS(t)
	config := contractTokenVerifyConfig()
	// This fixture retains signature/issuer/audience/expiry/access-token checks;
	// omitting the SDK required-user check proves the host boundary still rejects
	// alias-only identities instead of admitting an empty canonical principal.
	config.RequiredClaims = []string{"sub", "exp", "tenant_id"}
	verifier, err := auth.NewTokenVerifier(config, manager, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name               string
		user, account, org string
		extra              map[string]interface{}
		want               [3]string
		sources            [3]string
		status             int
	}{
		{name: "canonical beats all identity aliases", user: "11", account: "21", org: "31", extra: map[string]interface{}{"uid": "99", "organization_id": "98", "aid": "41", "service_id": "worker", "username": "alice"}, want: [3]string{"11", "21", "31"}, sources: [3]string{"canonical", "canonical", "canonical"}, status: 204},
		{name: "canonical only", user: "11", account: "21", org: "31", want: [3]string{"11", "21", "31"}, sources: [3]string{"canonical", "canonical", "canonical"}, status: 204},
		{name: "aliases alone rejected", extra: map[string]interface{}{"user_id": "11", "uid": "12", "account_id": "21", "accountId": "22", "org_id": "31", "organization_id": "32"}, sources: [3]string{"missing", "missing", "missing"}, status: 401},
		{name: "numeric aliases rejected", extra: map[string]interface{}{"uid": 12, "accountId": 22, "organization_id": 32}, sources: [3]string{"missing", "missing", "missing"}, status: 401},
		{name: "optional identity and organization cannot come from attributes", user: "11", extra: map[string]interface{}{"aid": "21", "org_id": "31"}, want: [3]string{"11", "", ""}, sources: [3]string{"canonical", "missing", "missing"}, status: 204},
		{name: "extensions alone rejected", extra: map[string]interface{}{"username": "alice", "service_id": "worker"}, sources: [3]string{"missing", "missing", "missing"}, status: 401},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fields := []string{"user_id", "account_id", "org_id"}
			before := map[string]map[string]float64{}
			fieldSources := map[string][]string{
				"user_id":    {"canonical", "missing"},
				"account_id": {"canonical", "missing"},
				"org_id":     {"canonical", "missing"},
			}
			for field, sources := range fieldSources {
				before[field] = map[string]float64{}
				for _, source := range sources {
					before[field][source] = metricValue(t, "qs_auth_claim_source_total", field, source)
				}
			}
			claims := contractClaims(time.Now().UTC(), "access")
			delete(claims, "user_id")
			if tc.user != "" {
				claims["user_id"] = tc.user
			}
			if tc.account != "" {
				claims["login_identity_id"] = tc.account
			}
			if tc.org != "" {
				claims["org_id"] = tc.org
			}
			claims["attributes"] = tc.extra
			token := signContractToken(t, privateKey, claims)
			engine := gin.New()
			engine.Use(middleware.JWTAuthMiddlewareWithOptions(verifier, nil))
			called := false
			engine.GET("/proof", func(c *gin.Context) {
				called = true
				mapped := middleware.GetUserClaims(c)
				got := [3]string{mapped.UserID, mapped.AccountID, mapped.OrgID}
				if got != tc.want {
					t.Errorf("mapped=%v want=%v", got, tc.want)
				}
				c.Status(http.StatusNoContent)
			})
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/proof", nil)
			request.Header.Set("Authorization", "Bearer "+token)
			engine.ServeHTTP(recorder, request)
			if recorder.Code != tc.status || called != (tc.status == http.StatusNoContent) {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body)
			}
			for index, field := range fields {
				for source, previous := range before[field] {
					wantDelta := float64(0)
					if source == tc.sources[index] {
						wantDelta = 1
					}
					if delta := metricValue(t, "qs_auth_claim_source_total", field, source) - previous; delta != wantDelta {
						t.Fatalf("%s/%s delta=%g want=%g", field, source, delta, wantDelta)
					}
				}
			}
		})
	}
}

func TestVerifiedGRPCCanonicalIdentityRejectsAliases(t *testing.T) {
	privateKey, manager := newContractJWKS(t)
	config := contractTokenVerifyConfig()
	config.RequiredClaims = []string{"sub", "exp", "tenant_id"}
	verifier, err := auth.NewTokenVerifier(config, manager, nil)
	if err != nil {
		t.Fatal(err)
	}
	interceptor := qsgrpc.NewIAMAuthInterceptor(verifier, &qsgrpc.AuthConfig{Enabled: true})
	for _, tc := range []struct {
		name, user, account, prefix string
		wantCode                    codes.Code
	}{
		{"canonical beats alias", "11", "21", "Bearer ", codes.OK},
		{"absent login identity ignores alias", "11", "", "Bearer ", codes.OK},
		{"alias-only user rejected", "", "", "Bearer ", codes.Unauthenticated},
		{"bare signed token rejected", "11", "21", "", codes.Unauthenticated},
	} {
		t.Run(tc.name, func(t *testing.T) {
			claims := contractClaims(time.Now().UTC(), "access")
			delete(claims, "user_id")
			if tc.user != "" {
				claims["user_id"] = tc.user
			}
			if tc.account != "" {
				claims["login_identity_id"] = tc.account
			}
			claims["sid"] = "session-proof"
			claims["attributes"] = map[string]interface{}{"user_id": "99", "uid": "98", "aid": "97", "account_id": "96", "username": "alice"}
			token := signContractToken(t, privateKey, claims)
			ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", tc.prefix+token))
			called := false
			_, err := interceptor.UnaryServerInterceptor()(ctx, nil, &ggrpc.UnaryServerInfo{FullMethod: "/proof.Canonical/Get"}, func(ctx context.Context, _ interface{}) (interface{}, error) {
				called = true
				if qsgrpc.UserIDFromContext(ctx) != tc.user || qsgrpc.AccountIDFromContext(ctx) != tc.account || qsgrpc.SessionIDFromContext(ctx) != "session-proof" || qsgrpc.UsernameFromContext(ctx) != "alice" {
					t.Fatal("canonical or session/optional-name projection changed")
				}
				return nil, nil
			})
			if status.Code(err) != tc.wantCode || called != (tc.wantCode == codes.OK) {
				t.Fatalf("code=%s called=%t err=%v", status.Code(err), called, err)
			}
		})
	}
}

func TestRESTTokenCarrierResultAndPriority(t *testing.T) {
	privateKey, manager := newContractJWKS(t)
	verifier, err := auth.NewTokenVerifier(contractTokenVerifyConfig(), manager, nil)
	if err != nil {
		t.Fatal(err)
	}
	token := signContractToken(t, privateKey, contractClaims(time.Now().UTC(), "access"))
	for _, tc := range []struct {
		name, header, query, cookie, source, result string
		status                                      int
	}{
		{name: "bearer dominates query and cookie", header: "Bearer " + token, query: "bad", cookie: "bad", source: "bearer", result: "accepted", status: 204},
		{name: "bare signed token is rejected", header: token, query: "bad", cookie: "bad", source: "bare", result: "rejected", status: 401},
		{name: "invalid bare never falls back", header: "bad", query: token, cookie: token, source: "bare", result: "rejected", status: 401},
		{name: "invalid bearer never falls back", header: "Bearer bad", query: token, source: "bearer", result: "rejected", status: 401},
		{name: "query retains precedence", query: token, cookie: "bad", source: "query", result: "accepted", status: 204},
		{name: "cookie retained", cookie: token, source: "cookie", result: "accepted", status: 204},
		{name: "empty bearer retains no fallback", header: "Bearer ", query: token, source: "bearer", result: "rejected", status: 401},
		{name: "missing", source: "missing", result: "rejected", status: 401},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := metricValue(t, "qs_auth_token_source_total", tc.source, tc.result)
			claimBefore := metricValue(t, "qs_auth_claim_source_total", "user_id", "canonical")
			engine := gin.New()
			engine.Use(middleware.JWTAuthMiddlewareWithOptions(verifier, nil))
			engine.GET("/proof", func(c *gin.Context) { c.Status(http.StatusNoContent) })
			request := httptest.NewRequest(http.MethodGet, "/proof", nil)
			request.Header.Set("Authorization", tc.header)
			q := request.URL.Query()
			q.Set("access_token", tc.query)
			request.URL.RawQuery = q.Encode()
			if tc.cookie != "" {
				request.AddCookie(&http.Cookie{Name: "access_token", Value: tc.cookie})
			}
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, request)
			if recorder.Code != tc.status {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body)
			}
			if delta := metricValue(t, "qs_auth_token_source_total", tc.source, tc.result) - before; delta != 1 {
				t.Fatalf("carrier delta=%g", delta)
			}
			delta := metricValue(t, "qs_auth_claim_source_total", "user_id", "canonical") - claimBefore
			if tc.result == "rejected" && delta != 0 {
				t.Fatalf("unverified claim observed: %g", delta)
			}
		})
	}
}

func TestAuthMetricsRegisterOnlyBoundedCanonicalSeries(t *testing.T) {
	if got := metricSeriesCount(t, "qs_auth_token_source_total"); got != 12 {
		t.Fatalf("token series=%d", got)
	}
	if got := metricSeriesCount(t, "qs_auth_claim_source_total"); got != 6 {
		t.Fatalf("claim series=%d", got)
	}
}

const (
	contractIssuer   = "https://iam.fangcunmount.cn"
	contractAudience = "qs-api"
	contractKeyID    = "qs-server-contract-key"
)

type staticContractKeyFetcher struct{ set jwk.Set }

func (f *staticContractKeyFetcher) Fetch(context.Context) (jwk.Set, error) { return f.set, nil }
func (*staticContractKeyFetcher) Name() string                             { return "static-contract-key" }
func contractTokenVerifyConfig() *sdk.TokenVerifyConfig {
	return &sdk.TokenVerifyConfig{
		AllowedIssuer:         contractIssuer,
		AllowedAudience:       []string{contractAudience},
		RequireExpirationTime: true,
		RequiredClaims:        []string{"sub", "exp", "user_id", "tenant_id"},
		Algorithms:            []string{"RS256"},
	}
}

func contractClaims(now time.Time, tokenType string) map[string]interface{} {
	return map[string]interface{}{
		jwt.SubjectKey:    "user:1001",
		jwt.IssuerKey:     contractIssuer,
		jwt.AudienceKey:   []string{contractAudience},
		jwt.IssuedAtKey:   now,
		jwt.ExpirationKey: now.Add(time.Minute),
		"user_id":         "1001",
		"tenant_id":       "fangcun",
		"token_type":      tokenType,
	}
}

func newContractJWKS(t *testing.T) (*rsa.PrivateKey, *authjwks.JWKSManager) {
	t.Helper()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	publicKey, err := jwk.FromRaw(&privateKey.PublicKey)
	if err != nil {
		t.Fatalf("jwk.FromRaw() error = %v", err)
	}
	if err := publicKey.Set(jwk.KeyIDKey, contractKeyID); err != nil {
		t.Fatalf("set key ID: %v", err)
	}
	if err := publicKey.Set(jwk.AlgorithmKey, jwa.RS256); err != nil {
		t.Fatalf("set algorithm: %v", err)
	}
	set := jwk.NewSet()
	if err := set.AddKey(publicKey); err != nil {
		t.Fatalf("AddKey() error = %v", err)
	}
	manager, err := authjwks.NewJWKSManager(
		&sdk.JWKSConfig{URL: "https://unused.invalid/.well-known/jwks.json"},
		authjwks.WithCustomChain(&staticContractKeyFetcher{set: set}),
		authjwks.WithCacheEnabled(false),
	)
	if err != nil {
		t.Fatalf("NewJWKSManager() error = %v", err)
	}
	t.Cleanup(manager.Stop)
	return privateKey, manager
}

func signContractToken(t *testing.T, privateKey *rsa.PrivateKey, claims map[string]interface{}) string {
	t.Helper()

	token := jwt.New()
	for name, value := range claims {
		if err := token.Set(name, value); err != nil {
			t.Fatalf("set claim %s: %v", name, err)
		}
	}
	headers := jws.NewHeaders()
	if err := headers.Set(jwk.KeyIDKey, contractKeyID); err != nil {
		t.Fatalf("set protected key ID: %v", err)
	}
	signed, err := jwt.Sign(token, jwt.WithKey(jwa.RS256, privateKey, jws.WithProtectedHeaders(headers)))
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}
	return string(signed)
}

func metricValue(t *testing.T, name, first, second string) float64 {
	t.Helper()
	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, family := range families {
		if family.GetName() != name {
			continue
		}
		for _, metric := range family.Metric {
			labels := map[string]string{}
			for _, label := range metric.Label {
				labels[label.GetName()] = label.GetValue()
			}
			firstLabel, secondLabel := "source", "result"
			if name == "qs_auth_claim_source_total" {
				firstLabel, secondLabel = "field", "source"
			}
			if labels[firstLabel] == first && labels[secondLabel] == second {
				return metric.GetCounter().GetValue()
			}
		}
	}
	t.Fatalf("missing preregistered metric %s/%s/%s", name, first, second)
	return 0
}
func metricSeriesCount(t *testing.T, name string) int {
	t.Helper()
	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, family := range families {
		if family.GetName() == name {
			return len(family.Metric)
		}
	}
	return 0
}
