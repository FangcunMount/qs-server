package compatibilityretirementfence

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// nativeLoopbackClient drives actual HTTP socket reads, using clearly local
// synthetic signed issuer/API fixtures. It does not claim GitHub/live-host proof.
type nativeLoopbackClient struct {
	client     *http.Client
	fixtureURL string
}

func (c nativeLoopbackClient) Do(original *http.Request) (*http.Response, error) {
	req := original.Clone(original.Context())
	parsed, err := url.Parse(c.fixtureURL + original.URL.RequestURI())
	if err != nil {
		return nil, ErrRemote
	}
	req.URL = parsed
	req.Host = ""
	response, err := c.client.Do(req)
	if response != nil {
		response.Request = original
	}
	return response, err
}

type nativePayload struct {
	Policy   Policy `json:"policy"`
	Token    string `json:"token"`
	Keys     []byte `json:"keys"`
	Key      string `json:"key"`
	Mutation string `json:"mutation"`
}
type nativeResult struct {
	Category     string  `json:"category"`
	HTTPRequests int     `json:"http_requests"`
	FixtureOnly  bool    `json:"fixture_only"`
	Receipt      Receipt `json:"receipt"`
}

// TestFenceNativeChild is a separate real Go process invoked by a fixed shell.
// Historical SSH_ORIGINAL_COMMAND is treated exclusively as data, never eval'd.
func TestFenceNativeChild(t *testing.T) {
	if os.Getenv("QS_FENCE_NATIVE_CHILD") != "1" {
		return
	}
	var in nativePayload
	raw, err := io.ReadAll(io.LimitReader(os.Stdin, 100000))
	if err != nil || decodeJSON(raw, &in) != nil {
		os.Exit(90)
	}
	fixture := &fixtureClient{p: in.Policy, keys: in.Keys, mutation: in.Mutation}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		original := r.Clone(r.Context())
		if r.Method != http.MethodGet || r.ContentLength != 0 {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		original.Body = nil // Incoming HTTP GET has http.NoBody; the actual upstream request has no body.
		if r.URL.Path == "/.well-known/jwks" {
			original.URL, _ = url.Parse(JWKSURL)
		} else {
			original.URL, _ = url.Parse(APIBase + r.URL.RequestURI())
		}
		response, e := fixture.Do(original)
		if e != nil {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		defer func() { _ = response.Body.Close() }()
		w.WriteHeader(response.StatusCode)
		if _, e = io.Copy(w, response.Body); e != nil {
			return
		}
	}))
	defer server.Close()
	permit, e := AuthorizeProbe(context.Background(), nativeLoopbackClient{server.Client(), server.URL}, in.Policy, os.Getenv("SSH_ORIGINAL_COMMAND"), in.Key, in.Token, in.Policy.LoginUID, time.Now())
	result := nativeResult{Category: "probe_authorized", HTTPRequests: fixture.calls, FixtureOnly: true, Receipt: permit.Receipt()}
	if e != nil {
		result.Category = e.Error()
	}
	// Raw stdout/stderr, JWT, private fixture metadata and payloads never enter errors.
	output, err := json.Marshal(result)
	if err != nil {
		os.Exit(91)
	}
	if _, err = os.Stdout.Write(append(output, '\n')); err != nil {
		os.Exit(92)
	}
	os.Exit(0)
}
func TestNativeShellRejectsHistoricalPayloadAndAdmitsOnlyBoundProbe(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal("native executable")
	}
	for _, name := range []string{"approved_probe", "old_bash_payload", "scp_payload", "sftp_subsystem", "command_injection", "old_main_sha", "historical_rerun", "queued", "waiting", "pending", "provider_error_privacy"} {
		t.Run(name, func(t *testing.T) {
			p := fixturePolicy()
			change := func(c map[string]any) {}
			if name == "old_main_sha" {
				change = func(c map[string]any) { c["workflow_sha"] = strings.Repeat("d", 40) }
			}
			if name == "historical_rerun" {
				change = func(c map[string]any) { c["run_attempt"] = "2" }
			}
			token, keys := fixtureToken(t, p, change)
			command := p.Command()
			mutation := ""
			switch name {
			case "old_bash_payload":
				command = "bash -s"
			case "scp_payload":
				command = "scp -t /tmp"
			case "sftp_subsystem":
				command = "internal-sftp"
			case "command_injection":
				command += "; printf forbidden-private-password"
			case "queued", "waiting", "pending":
				mutation = name
			case "provider_error_privacy":
				keys = []byte("private-provider-error-forbidden")
			}
			payload := mustJSON(t, nativePayload{Policy: p, Token: token, Keys: keys, Key: p.SSHKeyFingerprint, Mutation: mutation})
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, "/bin/sh", "-c", `exec "$1" -test.run='^TestFenceNativeChild$'`, "fence-owned-fixture", exe)
			cmd.Env = []string{"PATH=/usr/bin:/bin", "QS_FENCE_NATIVE_CHILD=1", "SSH_ORIGINAL_COMMAND=" + command}
			cmd.Stdin = bytes.NewReader(payload)
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			stdout, e := cmd.Output()
			if e != nil || stderr.Len() != 0 {
				t.Fatal("native child execution failed; raw output withheld")
			}
			if bytes.Contains(stdout, []byte(token)) || bytes.Contains(stdout, []byte("forbidden-private")) || bytes.Contains(stdout, []byte("private-provider-error")) {
				t.Fatal("private input leaked")
			}
			var result nativeResult
			if json.Unmarshal(stdout, &result) != nil || !result.FixtureOnly || result.Receipt.DropReady || result.Receipt.ProductionInstalled || result.Receipt.WholeSystemWriterFenceProven {
				t.Fatal("native output invalid or overclaims")
			}
			if name == "approved_probe" {
				if result.Category != "probe_authorized" || !result.Receipt.SSHProbeInvocationVerified || result.HTTPRequests < 20 {
					t.Fatal("bound probe did not read complete real HTTP fixture")
				}
			} else {
				if result.Category == "probe_authorized" || result.Receipt.SSHProbeInvocationVerified {
					t.Fatal("historical/unapproved native payload admitted")
				}
			}
		})
	}
}
