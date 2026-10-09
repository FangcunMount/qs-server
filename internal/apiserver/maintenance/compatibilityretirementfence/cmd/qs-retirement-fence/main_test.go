package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestHostOptionsRejectUnknownDuplicatedAndInjectedArguments(t *testing.T) {
	good := []string{"--policy", "/etc/qs-fence/policy.json", "--policy-sha256", "approved-hash", "--authenticated-key", "approved-key", "--github-token-file", "/etc/qs-fence/read-token"}
	if _, err := parseOptions(good); err != nil {
		t.Fatal("fixed host argv rejected")
	}
	for _, args := range [][]string{append(good, "private-unknown-value"), {"--policy", "x", "--policy", "y", "--authenticated-key", "k", "--github-token-file", "t"}, {"--unknown", "private-unknown-value", "--policy-sha256", "h", "--authenticated-key", "k", "--github-token-file", "t"}} {
		if _, err := parseOptions(args); err == nil || err.Error() != "retirement_fence_host_input_rejected" {
			t.Fatal("unsafe argv or raw error exposed")
		}
	}
	if _, err := run(context.Background(), []string{"--unknown-secret-value"}, bytes.NewReader([]byte("private-token")), "bash -s", uint32(os.Getuid())); err == nil || err.Error() != "retirement_fence_host_input_rejected" {
		t.Fatal("invalid input reached backend")
	}
}
func TestReadCredentialPrivateFileAndSymlinkRefusal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "read-token")
	if err := os.WriteFile(path, []byte("fixture-private-token\n"), 0600); err != nil {
		t.Fatal("fixture create")
	}
	token, err := readTokenFile(path, uint32(os.Getuid()))
	if err != nil || !bytes.Equal(token, []byte("fixture-private-token")) {
		t.Fatal("host private credential unavailable")
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal("fixture chmod")
	}
	if _, err = readTokenFile(path, uint32(os.Getuid())); err == nil {
		t.Fatal("world-readable credential accepted")
	}
	alias := filepath.Join(dir, "alias")
	if err = os.Symlink(path, alias); err != nil {
		t.Fatal("fixture symlink")
	}
	if _, err = readTokenFile(alias, uint32(os.Getuid())); err == nil {
		t.Fatal("symlink credential accepted")
	}
}

type captureTransport struct {
	auth  string
	calls int
}

func (c *captureTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	c.auth = r.Header.Get("Authorization")
	c.calls++
	return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(nil)), Request: r}, nil
}
func TestReadCredentialNeverSentToIssuerOrOtherOrigin(t *testing.T) {
	capture := &captureTransport{}
	transport := readAuthTransport{token: []byte("fixture-private-token"), inner: capture}
	req, _ := http.NewRequest(http.MethodGet, "https://token.actions.githubusercontent.com/.well-known/jwks", nil)
	resp, err := transport.RoundTrip(req)
	if err != nil || capture.auth != "" {
		t.Fatal("credential sent to issuer")
	}
	if err = resp.Body.Close(); err != nil {
		t.Fatal("fixture close")
	}
	req, _ = http.NewRequest(http.MethodGet, "https://api.github.com/repos/FangcunMount/qs-server", nil)
	resp, err = transport.RoundTrip(req)
	if err != nil || capture.auth != "Bearer fixture-private-token" {
		t.Fatal("fixed API credential missing")
	}
	if err = resp.Body.Close(); err != nil {
		t.Fatal("fixture close")
	}
	for _, address := range []string{"https://other.invalid/", "http://api.github.com/", "https://api.github.com@other.invalid/"} {
		req, _ = http.NewRequest(http.MethodGet, address, nil)
		if _, err = transport.RoundTrip(req); err == nil {
			t.Fatal("untrusted origin reached transport")
		}
	}
	if capture.calls != 2 {
		t.Fatal("untrusted origin called transport")
	}
}
