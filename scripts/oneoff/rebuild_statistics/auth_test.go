package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestJWTExpiresAt(t *testing.T) {
	want := time.Now().Add(time.Hour).Truncate(time.Second)
	got := jwtExpiresAt(testJWT(t, want))
	if !got.Equal(want) {
		t.Fatalf("expires_at=%s want=%s", got, want)
	}
	if got := jwtExpiresAt("not-a-jwt"); !got.IsZero() {
		t.Fatalf("invalid JWT expiry=%s, want zero", got)
	}
}

func TestReadSecretFileRequiresPrivateRegularFile(t *testing.T) {
	path := writePasswordFile(t, "correct horse battery staple")
	got, err := readSecretFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != "correct horse battery staple" {
		t.Fatalf("secret=%q", got)
	}

	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readSecretFile(path); err == nil || !strings.Contains(err.Error(), "permissions") {
		t.Fatalf("err=%v, want permissions error", err)
	}

	link := filepath.Join(t.TempDir(), "password-link")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if _, err := readSecretFile(link); err == nil || !strings.Contains(err.Error(), "non-symlink") {
		t.Fatalf("err=%v, want symlink error", err)
	}
}

func TestIAMTokenSourceLogsInProactivelyAndReusesToken(t *testing.T) {
	passwordFile := writePasswordFile(t, "iam-password")
	freshToken := testJWT(t, time.Now().Add(time.Hour))
	var loginRequests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/authn/login" {
			http.NotFound(w, r)
			return
		}
		loginRequests.Add(1)
		var request struct {
			AuthMethod    string `json:"auth_method"`
			DeviceID      string `json:"device_id"`
			MethodPayload struct {
				Username string `json:"username"`
				Password string `json:"password"`
				TenantID uint64 `json:"tenant_id"`
			} `json:"method_payload"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			return
		}
		if request.AuthMethod != "password" || request.DeviceID != statisticsDeviceID ||
			request.MethodPayload.Username != "system@example.com" || request.MethodPayload.Password != "iam-password" ||
			request.MethodPayload.TenantID != 7 {
			t.Errorf("unexpected IAM login request: %+v", request)
		}
		writeIAMLoginResponse(t, w, freshToken)
	}))
	defer server.Close()

	var output bytes.Buffer
	source, err := newBearerTokenSource(options{
		IAMLoginURL: server.URL + "/api/v2/authn/login", IAMUsername: "system@example.com",
		IAMPasswordFile: passwordFile, IAMTenantID: 7, IAMRefreshSkew: 2 * time.Minute,
	}, &output)
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 2; index++ {
		got, err := source.Token(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if got != freshToken {
			t.Fatalf("token=%q want=%q", got, freshToken)
		}
	}
	if got := loginRequests.Load(); got != 1 {
		t.Fatalf("login requests=%d want=1", got)
	}
	if strings.Contains(output.String(), freshToken) || strings.Contains(output.String(), "iam-password") {
		t.Fatalf("secret leaked in output: %s", output.String())
	}
	if !strings.Contains(output.String(), "reason=proactive") || !strings.Contains(output.String(), "expires_at=") {
		t.Fatalf("missing safe refresh diagnostic: %s", output.String())
	}
}

func TestIAMTokenSourceRefreshesBeforeJWTExpiryWindow(t *testing.T) {
	passwordFile := writePasswordFile(t, "iam-password")
	nearExpiryToken := testJWT(t, time.Now().Add(30*time.Second))
	freshToken := testJWT(t, time.Now().Add(time.Hour))
	var loginRequests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		loginRequests.Add(1)
		writeIAMLoginResponse(t, w, freshToken)
	}))
	defer server.Close()

	source, err := newBearerTokenSource(options{
		Token: nearExpiryToken, IAMLoginURL: server.URL, IAMUsername: "system@example.com",
		IAMPasswordFile: passwordFile, IAMRefreshSkew: 2 * time.Minute,
	}, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	got, err := source.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got != freshToken || loginRequests.Load() != 1 {
		t.Fatalf("token=%q login_requests=%d", got, loginRequests.Load())
	}
}

func TestExecuteRunRefreshesOnceAfterUnauthorized(t *testing.T) {
	passwordFile := writePasswordFile(t, "iam-password")
	initialToken := testJWT(t, time.Now().Add(time.Hour))
	freshToken := testJWT(t, time.Now().Add(2*time.Hour))
	var loginRequests atomic.Int64
	var statisticsRequests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/authn/login":
			loginRequests.Add(1)
			writeIAMLoginResponse(t, w, freshToken)
		case "/internal/v2/statistics/runs":
			requestNumber := statisticsRequests.Add(1)
			if requestNumber == 1 {
				if got := r.Header.Get("Authorization"); got != "Bearer "+initialToken {
					t.Errorf("initial Authorization=%q", got)
				}
				http.Error(w, "expired", http.StatusUnauthorized)
				return
			}
			if got := r.Header.Get("Authorization"); got != "Bearer "+freshToken {
				t.Errorf("refreshed Authorization=%q", got)
			}
			_, _ = w.Write([]byte(`{"code":0,"data":{"id":99,"mode":"repair","status":"succeeded","stage":"completed"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	var output bytes.Buffer
	cfg := options{
		BaseURL: server.URL, Token: initialToken, Mode: "repair", Reason: "test", Confirm: true,
		IAMLoginURL: server.URL, IAMUsername: "system@example.com", IAMPasswordFile: passwordFile,
		IAMRefreshSkew: 2 * time.Minute,
	}
	var err error
	cfg.TokenSource, err = newBearerTokenSource(cfg, &output)
	if err != nil {
		t.Fatal(err)
	}
	location, _ := time.LoadLocation("Asia/Shanghai")
	result, err := executeRun(server.Client(), cfg, 1, dateWindow{
		From: time.Date(2026, 1, 1, 0, 0, 0, 0, location),
		To:   time.Date(2026, 1, 7, 0, 0, 0, 0, location),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ID != 99 || statisticsRequests.Load() != 2 || loginRequests.Load() != 1 {
		t.Fatalf("result=%+v statistics_requests=%d login_requests=%d", result, statisticsRequests.Load(), loginRequests.Load())
	}
	if !strings.Contains(output.String(), "reason=unauthorized") {
		t.Fatalf("output=%s", output.String())
	}
}

func TestRunUsesIAMAuthenticationWithoutStaticToken(t *testing.T) {
	t.Setenv("QS_STATISTICS_TOKEN", "")
	t.Setenv("QS_STATISTICS_IAM_LOGIN_URL", "")
	t.Setenv("QS_STATISTICS_IAM_USERNAME", "")
	t.Setenv("QS_STATISTICS_IAM_PASSWORD_FILE", "")
	t.Setenv("QS_STATISTICS_IAM_TENANT_ID", "")
	t.Setenv("QS_STATISTICS_IAM_REFRESH_SKEW", "")

	passwordFile := writePasswordFile(t, "iam-password")
	freshToken := testJWT(t, time.Now().Add(time.Hour))
	var loginRequests atomic.Int64
	var statisticsRequests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/authn/login":
			loginRequests.Add(1)
			writeIAMLoginResponse(t, w, freshToken)
		case "/internal/v2/statistics/runs":
			statisticsRequests.Add(1)
			if got := r.Header.Get("Authorization"); got != "Bearer "+freshToken {
				t.Errorf("Authorization=%q", got)
			}
			_, _ = w.Write([]byte(`{"code":0,"data":{"id":100,"mode":"validate","status":"succeeded","stage":"completed","fact_counts":{"access.inserted":0,"access.conflict":0,"plan.inserted":0,"plan.conflict":0,"assessment.inserted":0,"assessment.conflict":0}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	err := run([]string{
		"--base-url", server.URL,
		"--iam-login-url", server.URL + "/api/v2/authn/login",
		"--iam-username", "system@example.com",
		"--iam-password-file", passwordFile,
		"--org-ids", "1",
		"--from", "2026-01-01",
		"--to", "2026-01-01",
		"--validate-only",
	}, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if loginRequests.Load() != 1 || statisticsRequests.Load() != 1 {
		t.Fatalf("login_requests=%d statistics_requests=%d", loginRequests.Load(), statisticsRequests.Load())
	}
}

func TestExecuteRunDoesNotRefreshAfterForbidden(t *testing.T) {
	passwordFile := writePasswordFile(t, "iam-password")
	initialToken := testJWT(t, time.Now().Add(time.Hour))
	var loginRequests atomic.Int64
	var statisticsRequests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/authn/login" {
			loginRequests.Add(1)
			writeIAMLoginResponse(t, w, testJWT(t, time.Now().Add(2*time.Hour)))
			return
		}
		statisticsRequests.Add(1)
		http.Error(w, "forbidden", http.StatusForbidden)
	}))
	defer server.Close()

	cfg := options{
		BaseURL: server.URL, Token: initialToken, Mode: "repair", Reason: "test", Confirm: true,
		IAMLoginURL: server.URL, IAMUsername: "system@example.com", IAMPasswordFile: passwordFile,
		IAMRefreshSkew: 2 * time.Minute,
	}
	var err error
	cfg.TokenSource, err = newBearerTokenSource(cfg, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	location, _ := time.LoadLocation("Asia/Shanghai")
	_, err = executeRun(server.Client(), cfg, 1, dateWindow{
		From: time.Date(2026, 1, 1, 0, 0, 0, 0, location),
		To:   time.Date(2026, 1, 7, 0, 0, 0, 0, location),
	})
	if err == nil || !strings.Contains(err.Error(), "403 Forbidden") {
		t.Fatalf("err=%v", err)
	}
	if statisticsRequests.Load() != 1 || loginRequests.Load() != 0 {
		t.Fatalf("statistics_requests=%d login_requests=%d", statisticsRequests.Load(), loginRequests.Load())
	}
}

func TestAuthenticationConfigurationSupportsIAMOrStaticToken(t *testing.T) {
	if err := (options{Token: "static"}).validateAuthentication(); err != nil {
		t.Fatal(err)
	}
	if err := (options{
		IAMLoginURL: "https://iam.example.com/api/v2/authn/login", IAMUsername: "system@example.com",
		IAMPasswordFile: "/secure/iam-password", IAMRefreshSkew: time.Minute,
	}).validateAuthentication(); err != nil {
		t.Fatal(err)
	}
	if err := (options{IAMLoginURL: "https://iam.example.com"}).validateAuthentication(); err == nil || !strings.Contains(err.Error(), "configured together") {
		t.Fatalf("err=%v", err)
	}
	if err := (options{}).validateAuthentication(); err == nil || !strings.Contains(err.Error(), "token or complete IAM") {
		t.Fatalf("err=%v", err)
	}
}

func TestIAMLoginClientBaseURL(t *testing.T) {
	for input, want := range map[string]string{
		"https://iam.example.com/api/v2/authn/login":        "https://iam.example.com",
		"https://iam.example.com/prefix/api/v2/authn/login": "https://iam.example.com/prefix",
		"https://iam.example.com/api/v2":                    "https://iam.example.com",
		"https://iam.example.com":                           "https://iam.example.com",
	} {
		got, err := iamLoginClientBaseURL(input)
		if err != nil {
			t.Fatalf("input=%s err=%v", input, err)
		}
		if got != want {
			t.Fatalf("input=%s got=%s want=%s", input, got, want)
		}
	}
}

func writePasswordFile(t *testing.T, password string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "iam-password")
	if err := os.WriteFile(path, []byte(password+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func testJWT(t *testing.T, expiresAt time.Time) string {
	t.Helper()
	payload, err := json.Marshal(map[string]int64{"exp": expiresAt.Unix()})
	if err != nil {
		t.Fatal(err)
	}
	return "e30." + base64.RawURLEncoding.EncodeToString(payload) + ".signature"
}

func writeIAMLoginResponse(t *testing.T, w http.ResponseWriter, token string) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{
		"code": 0,
		"data": map[string]any{
			"access_token": token,
			"token_type":   "Bearer",
			"expires_in":   3600,
		},
	}); err != nil {
		t.Error(err)
	}
}
