package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/FangcunMount/iam/v2/pkg/sdk/auth/loginv2"
)

const (
	defaultIAMLoginTimeout  = 15 * time.Second
	defaultTokenRefreshSkew = 2 * time.Minute
	statisticsDeviceID      = "qs-statistics-rebuild"
)

type bearerTokenSource interface {
	Token(context.Context) (string, error)
	Refresh(context.Context, string) (string, error)
	Refreshable() bool
}

type iamBearerTokenSource struct {
	mu           sync.Mutex
	client       *loginv2.Client
	username     string
	passwordFile string
	tenantID     uint64
	refreshSkew  time.Duration
	output       io.Writer
	now          func() time.Time
	token        string
	expiresAt    time.Time
}

func newBearerTokenSource(cfg options, output io.Writer) (bearerTokenSource, error) {
	if !cfg.iamConfigured() {
		return nil, nil
	}
	if _, err := readSecretFile(cfg.IAMPasswordFile); err != nil {
		return nil, fmt.Errorf("validate IAM password file: %w", err)
	}
	baseURL, err := iamLoginClientBaseURL(cfg.IAMLoginURL)
	if err != nil {
		return nil, err
	}
	client, err := loginv2.NewClient(baseURL, loginv2.WithHTTPClient(&http.Client{Timeout: defaultIAMLoginTimeout}))
	if err != nil {
		return nil, fmt.Errorf("create IAM login client: %w", err)
	}
	source := &iamBearerTokenSource{
		client:       client,
		username:     strings.TrimSpace(cfg.IAMUsername),
		passwordFile: strings.TrimSpace(cfg.IAMPasswordFile),
		tenantID:     cfg.IAMTenantID,
		refreshSkew:  cfg.IAMRefreshSkew,
		output:       output,
		now:          time.Now,
		token:        strings.TrimSpace(cfg.Token),
	}
	source.expiresAt = jwtExpiresAt(source.token)
	return source, nil
}

func (s *iamBearerTokenSource) Token(ctx context.Context) (string, error) {
	if s == nil {
		return "", errors.New("IAM token source is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.tokenUsableLocked() {
		return s.token, nil
	}
	return s.loginLocked(ctx, "proactive")
}

func (s *iamBearerTokenSource) Refresh(ctx context.Context, staleToken string) (string, error) {
	if s == nil {
		return "", errors.New("IAM token source is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.token != "" && s.token != strings.TrimSpace(staleToken) && s.tokenUsableLocked() {
		return s.token, nil
	}
	return s.loginLocked(ctx, "unauthorized")
}

func (s *iamBearerTokenSource) Refreshable() bool { return s != nil }

func (s *iamBearerTokenSource) tokenUsableLocked() bool {
	if strings.TrimSpace(s.token) == "" || s.expiresAt.IsZero() {
		return false
	}
	now := time.Now()
	if s.now != nil {
		now = s.now()
	}
	return now.Add(s.refreshSkew).Before(s.expiresAt)
}

func (s *iamBearerTokenSource) loginLocked(ctx context.Context, reason string) (string, error) {
	password, err := readSecretFile(s.passwordFile)
	if err != nil {
		return "", fmt.Errorf("read IAM password: %w", err)
	}
	pair, err := s.client.Login(ctx, loginv2.LoginRequest{
		AuthMethod: loginv2.AuthMethodPassword,
		MethodPayload: loginv2.PasswordPayload{
			Username: s.username,
			Password: password,
			TenantID: s.tenantID,
		},
		DeviceID: statisticsDeviceID,
	})
	if err != nil {
		return "", fmt.Errorf("refresh Statistics bearer token from IAM: %w", err)
	}
	token := strings.TrimSpace(pair.AccessToken)
	if token == "" {
		return "", errors.New("IAM login response has no access token")
	}
	now := time.Now()
	if s.now != nil {
		now = s.now()
	}
	expiresAt := jwtExpiresAt(token)
	if expiresAt.IsZero() && pair.ExpiresIn > 0 {
		expiresAt = now.Add(time.Duration(pair.ExpiresIn) * time.Second)
	}
	if expiresAt.IsZero() || !expiresAt.After(now.Add(s.refreshSkew)) {
		return "", fmt.Errorf("IAM access token expiry is missing or too close: expires_at=%s", expiresAt.Format(time.RFC3339))
	}
	s.token, s.expiresAt = token, expiresAt
	if s.output != nil {
		_, _ = fmt.Fprintf(s.output, "statistics auth: IAM token refreshed reason=%s expires_at=%s\n", reason, expiresAt.Format(time.RFC3339))
	}
	return token, nil
}

func readSecretFile(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", errors.New("secret file path is empty")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return "", errors.New("secret file must be a regular non-symlink file")
	}
	if info.Mode().Perm()&0o077 != 0 {
		return "", fmt.Errorf("secret file permissions must not grant group/other access: mode=%04o", info.Mode().Perm())
	}
	value, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	secret := strings.TrimSuffix(string(value), "\n")
	secret = strings.TrimSuffix(secret, "\r")
	if secret == "" {
		return "", errors.New("secret file is empty")
	}
	if strings.ContainsAny(secret, "\r\n") {
		return "", errors.New("secret file must contain exactly one line")
	}
	return secret, nil
}

func jwtExpiresAt(token string) time.Time {
	parts := strings.Split(strings.TrimSpace(token), ".")
	if len(parts) < 2 {
		return time.Time{}
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return time.Time{}
	}
	var claims struct {
		ExpiresAt json.Number `json:"exp"`
	}
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.UseNumber()
	if err := decoder.Decode(&claims); err != nil || claims.ExpiresAt == "" {
		return time.Time{}
	}
	seconds, err := claims.ExpiresAt.Int64()
	if err != nil || seconds <= 0 {
		return time.Time{}
	}
	return time.Unix(seconds, 0)
}

func iamLoginClientBaseURL(loginURL string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(loginURL))
	if err != nil {
		return "", fmt.Errorf("parse IAM login URL: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("IAM login URL must be absolute")
	}
	path := strings.TrimRight(parsed.Path, "/")
	switch {
	case strings.HasSuffix(path, "/api/v2/authn/login"):
		path = strings.TrimSuffix(path, "/api/v2/authn/login")
	case strings.HasSuffix(path, "/api/v2"):
		path = strings.TrimSuffix(path, "/api/v2")
	case strings.HasSuffix(path, "/authn/login"):
		path = strings.TrimSuffix(path, "/authn/login")
	}
	parsed.Path = path
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}
