package compatibilityretirementfence

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

type fixtureClient struct {
	p         Policy
	keys      []byte
	mutation  string
	snapshots int
	calls     int
}

func fixturePolicy() Policy {
	ids := append([]int64(nil), historicalWorkflowIDs...)
	for i := range currentWorkflowPaths {
		ids = append(ids, int64(5000+i))
	}
	now := time.Now().UTC().Truncate(time.Second)
	p := Policy{Version: 1, RepositoryID: "100", OwnerID: "200", SourceSHA: strings.Repeat("a", 40), OperationID: "900-1", RequestSHA256: strings.Repeat("b", 64), RunID: "900", RunAttempt: "1", WorkflowID: 5011, CheckRunID: "800", ActorID: "300", RunnerID: 400, Subject: "repo:FangcunMount/qs-server:environment:production", LoginUID: 1234, SSHKeyFingerprint: "SHA256:" + strings.Repeat("a", 43), ChallengeSHA256: strings.Repeat("c", 64), ExpiresAt: now.Add(10 * time.Minute), WorkflowIDs: ids}
	p.Audience = p.ExpectedAudience()
	return p
}
func fixtureToken(t *testing.T, p Policy, change func(map[string]any)) (string, []byte) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal("fixture key generation")
	}
	now := time.Now().Unix()
	claims := map[string]any{"iss": Issuer, "aud": p.Audience, "sub": p.Subject, "exp": now + 180, "iat": now - 1, "nbf": now - 1, "jti": "fixture-one-token", "repository": Repository, "repository_id": p.RepositoryID, "repository_owner_id": p.OwnerID, "run_id": p.RunID, "run_attempt": p.RunAttempt, "actor_id": p.ActorID, "check_run_id": p.CheckRunID, "ref": "refs/heads/main", "ref_type": "branch", "head_ref": "", "base_ref": "", "workflow_ref": Repository + "/" + WorkflowPath + "@refs/heads/main", "workflow_sha": p.SourceSHA, "sha": p.SourceSHA, "event_name": "workflow_dispatch", "environment": "production", "runner_environment": "self-hosted"}
	if change != nil {
		change(claims)
	}
	token := fixtureSign(t, key, claims)
	enc := base64.RawURLEncoding
	keys, _ := json.Marshal(jwks{Keys: []jwk{{Kid: "fixture-key", Kty: "RSA", Alg: "RS256", Use: "sig", N: enc.EncodeToString(key.N.Bytes()), E: "AQAB"}}})
	return token, keys
}
func fixtureSign(t *testing.T, key *rsa.PrivateKey, claims any) string {
	t.Helper()
	enc := base64.RawURLEncoding
	h, _ := json.Marshal(map[string]string{"alg": "RS256", "kid": "fixture-key", "typ": "JWT"})
	b, _ := json.Marshal(claims)
	wire := enc.EncodeToString(h) + "." + enc.EncodeToString(b)
	sum := sha256.Sum256([]byte(wire))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if err != nil {
		t.Fatal("fixture signing")
	}
	return wire + "." + enc.EncodeToString(sig)
}
func (f *fixtureClient) Do(req *http.Request) (*http.Response, error) {
	f.calls++
	if req.Method != http.MethodGet || req.Body != nil {
		return nil, errors.New("private-provider-error-do-not-leak")
	}
	var value any
	p := f.p
	repo := githubRepository{ID: 100, FullName: Repository}
	repo.Owner.ID = 200
	run := githubRun{ID: 900, RunAttempt: 1, WorkflowID: p.WorkflowID, HeadSHA: p.SourceSHA, HeadBranch: "main", Event: "workflow_dispatch", Status: "in_progress", Path: WorkflowPath, Repository: repo, HeadRepository: repo}
	run.Actor.ID = 300
	run.TriggeringActor.ID = 300
	switch {
	case req.URL.String() == JWKSURL:
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(f.keys)), Request: req}, nil
	case req.URL.Path == "/repos/"+Repository:
		value = repo
	case strings.HasSuffix(req.URL.Path, "/commits/main"):
		sha := p.SourceSHA
		if f.mutation == "main_advanced" {
			sha = strings.Repeat("d", 40)
		}
		value = map[string]any{"sha": sha}
	case strings.HasSuffix(req.URL.Path, "/actions/runs/900/attempts/1"):
		f.snapshots++
		if f.mutation == "attempt_changed" {
			run.RunAttempt = 2
		}
		if f.mutation == "old_main_sha" {
			run.HeadSHA = strings.Repeat("d", 40)
		}
		value = run
	case strings.HasSuffix(req.URL.Path, "/actions/workflows"):
		rows := []githubWorkflow{}
		for _, id := range historicalWorkflowIDs {
			rows = append(rows, githubWorkflow{ID: id, Path: ".github/workflows/historical-" + canonicalInteger(id) + ".yml", State: "disabled_manually"})
		}
		for i, path := range currentWorkflowPaths {
			rows = append(rows, githubWorkflow{ID: int64(5000 + i), Path: ".github/workflows/" + path, State: "active"})
		}
		known := map[int64]bool{}
		for _, row := range rows {
			known[row.ID] = true
		}
		for _, id := range p.WorkflowIDs {
			if !known[id] {
				rows = append(rows, githubWorkflow{ID: id, Path: ".github/workflows/additional-" + canonicalInteger(id) + ".yml", State: "active"})
			}
		}
		total := len(rows)
		if f.mutation == "catalog_incomplete" {
			total++
		}
		page, _ := strconv.Atoi(req.URL.Query().Get("page"))
		start := (page - 1) * 100
		if start < 0 || start > len(rows) {
			return nil, ErrRemote
		}
		end := start + 100
		if end > len(rows) {
			end = len(rows)
		}
		value = map[string]any{"total_count": total, "workflows": rows[start:end]}
	case strings.HasSuffix(req.URL.Path, "/actions/runs"):
		status := req.URL.Query().Get("status")
		rows := []githubRun{}
		if status == "in_progress" {
			rows = append(rows, run)
		}
		if f.mutation == status || f.mutation == "unknown_workflow" && status == "waiting" {
			other := run
			other.ID = 901
			other.Status = status
			other.WorkflowID = historicalWorkflowIDs[0]
			if f.mutation == "unknown_workflow" {
				other.WorkflowID = 1
			}
			rows = append(rows, other)
		}
		value = map[string]any{"total_count": len(rows), "workflow_runs": rows}
	case strings.HasSuffix(req.URL.Path, "/attempts/1/jobs"):
		job := githubJob{ID: 700, RunID: 900, HeadSHA: p.SourceSHA, Status: "in_progress", CheckRunURL: APIBase + "/repos/" + Repository + "/check-runs/800", RunnerID: 400}
		if f.mutation == "runner_changed" {
			job.RunnerID++
		}
		if f.mutation == "changed_between_snapshots" && f.snapshots > 1 {
			job.ID++
		}
		value = map[string]any{"total_count": 1, "jobs": []githubJob{job}}
	default:
		return nil, ErrRemote
	}
	b, _ := json.Marshal(value)
	return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(b)), Request: req}, nil
}
func TestProbeCryptographicOriginAndTwoCompleteSnapshots(t *testing.T) {
	p := fixturePolicy()
	token, keys := fixtureToken(t, p, nil)
	client := &fixtureClient{p: p, keys: keys}
	permit, err := AuthorizeProbe(context.Background(), client, p, p.Command(), p.SSHKeyFingerprint, token, p.LoginUID, time.Now())
	if err != nil {
		t.Fatal("approved fixture denied", err)
	}
	r := permit.Receipt()
	if !r.SSHProbeInvocationVerified || r.WholeSystemWriterFenceProven || r.ProductionInstalled || r.DropReady || r.MutationBackendEnabled || client.snapshots != 2 || r.WorkflowCount != 21 {
		t.Fatal("fixture receipt overclaims")
	}
	if bytes.Contains(mustJSON(t, r), []byte(token)) {
		t.Fatal("raw token leaked")
	}
}
func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, e := json.Marshal(v)
	if e != nil {
		t.Fatal("fixture marshal")
	}
	return b
}
func TestOriginRefRunAttemptAndTamperingDenied(t *testing.T) {
	cases := map[string]func(map[string]any){"old_sha": func(c map[string]any) { c["workflow_sha"] = strings.Repeat("d", 40) },
		"event_sha_mismatch": func(c map[string]any) { c["sha"] = strings.Repeat("d", 40) },
		"missing_event_sha":  func(c map[string]any) { delete(c, "sha") }, "old_ref": func(c map[string]any) { c["ref"] = "refs/tags/old" }, "historical_main_rerun": func(c map[string]any) { c["run_attempt"] = "2" }, "other_run": func(c map[string]any) { c["run_id"] = "901" }, "wrong_org": func(c map[string]any) { c["repository_owner_id"] = "201" }, "other_job": func(c map[string]any) { c["check_run_id"] = "801" }, "expired": func(c map[string]any) { c["exp"] = time.Now().Unix() - 1 }, "other_audience": func(c map[string]any) { c["aud"] = "unapproved" }, "other_actor": func(c map[string]any) { c["actor_id"] = "301" }, "wrong_environment": func(c map[string]any) { c["environment"] = "staging" }, "not_yet_valid": func(c map[string]any) { c["nbf"] = time.Now().Unix() + 3600 }}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			p := fixturePolicy()
			token, keys := fixtureToken(t, p, change)
			f := &fixtureClient{p: p, keys: keys}
			if _, e := AuthorizeProbe(context.Background(), f, p, p.Command(), p.SSHKeyFingerprint, token, p.LoginUID, time.Now()); !errors.Is(e, ErrToken) || f.snapshots != 0 {
				t.Fatal("origin was not rejected before platform/DB")
			}
		})
	}
	p := fixturePolicy()
	token, keys := fixtureToken(t, p, nil)
	parts := strings.Split(token, ".")
	raw, _ := base64.RawURLEncoding.DecodeString(parts[1])
	raw = bytes.Replace(raw, []byte(`"run_id":"900"`), []byte(`"run_id":"901"`), 1)
	parts[1] = base64.RawURLEncoding.EncodeToString(raw)
	if _, e := verifyOIDC(strings.Join(parts, "."), keys, p, time.Now()); !errors.Is(e, ErrToken) {
		t.Fatal("tampered signature accepted")
	}
}
func TestQueueCoverageAndRecheckReject(t *testing.T) {
	for _, name := range []string{"queued", "waiting", "pending", "requested", "in_progress", "unknown_workflow", "catalog_incomplete", "main_advanced", "attempt_changed", "old_main_sha", "runner_changed", "changed_between_snapshots"} {
		t.Run(name, func(t *testing.T) {
			p := fixturePolicy()
			token, keys := fixtureToken(t, p, nil)
			f := &fixtureClient{p: p, keys: keys, mutation: name}
			if _, e := AuthorizeProbe(context.Background(), f, p, p.Command(), p.SSHKeyFingerprint, token, p.LoginUID, time.Now()); e == nil {
				t.Fatal("unsafe platform snapshot accepted")
			}
		})
	}
}
func TestHistoricalCommandAndClientIdentityCannotReachAPI(t *testing.T) {
	p := fixturePolicy()
	f := &fixtureClient{p: p}
	for _, command := range []string{"bash -s", "scp -t /tmp", "/usr/lib/openssh/sftp-server", p.Command() + "; bash", strings.Replace(p.Command(), p.SourceSHA, strings.Repeat("d", 40), 1), ""} {
		if _, e := AuthorizeProbe(context.Background(), f, p, command, p.SSHKeyFingerprint, "raw-secret", p.LoginUID, time.Now()); !errors.Is(e, ErrCommand) {
			t.Fatal("historical payload accepted")
		}
	}
	if _, e := AuthorizeProbe(context.Background(), f, p, p.Command(), "SHA256:"+strings.Repeat("b", 43), "raw-secret", p.LoginUID, time.Now()); !errors.Is(e, ErrIdentity) {
		t.Fatal("wrong key accepted")
	}
	if f.calls != 0 {
		t.Fatal("rejected commands reached network")
	}
}
func TestSSHRestrictionAndExactOriginalRollback(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal("fixture key")
	}
	public, err := ssh.NewPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal("fixture public key")
	}
	original := append([]byte("# original settings\nfrom=\"127.0.0.1\",command=\"bash -s\" "), ssh.MarshalAuthorizedKey(public)...)
	f := ssh.FingerprintSHA256(public)
	plan, err := PrepareSSH(original, digest(original), "/opt/qs-fence/gate", "/etc/qs-fence/policy.json", strings.Repeat("a", 64), "/etc/qs-fence/api-read-token", []string{f})
	if err != nil {
		t.Fatal("restriction preparation", err)
	}
	restricted := plan.RestrictedBytes()
	_, _, opts, rest, err := ssh.ParseAuthorizedKey(restricted)
	if err != nil || len(rest) != 0 || len(opts) != 3 || opts[0] != "restrict" || opts[1] != `from="127.0.0.1"` || !strings.Contains(opts[2], "--authenticated-key "+f) {
		t.Fatal("restriction changed origin or key")
	}
	restored, err := plan.RestoreOriginal(restricted)
	if err != nil || !bytes.Equal(restored, original) {
		t.Fatal("exact original rollback failed")
	}
	if _, err = plan.RestoreOriginal(append(restricted, byte('x'))); !errors.Is(err, ErrChanged) {
		t.Fatal("concurrent config overwritten")
	}
	if plan.Receipt().ProductionInstalled {
		t.Fatal("source planning called installed")
	}
	if _, err = PrepareSSH(original, digest(original), "/opt/qs-fence/gate;bash", "/etc/qs-fence/policy.json", strings.Repeat("a", 64), "/etc/qs-fence/api-read-token", []string{f}); !errors.Is(err, ErrSSH) {
		t.Fatal("injected gate path accepted")
	}
	if _, err = PrepareSSH(append([]byte("environment=\"LD_PRELOAD=bad\" "), ssh.MarshalAuthorizedKey(public)...), digest(append([]byte("environment=\"LD_PRELOAD=bad\" "), ssh.MarshalAuthorizedKey(public)...)), "/opt/qs-fence/gate", "/etc/qs-fence/policy.json", strings.Repeat("a", 64), "/etc/qs-fence/api-read-token", []string{f}); !errors.Is(err, ErrSSH) {
		t.Fatal("environment key option accepted")
	}
}
func TestProtectedPolicyRejectsWritableParentsSymlinksAndUnknownFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "policy.json")
	raw := mustJSON(t, fixturePolicy())
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal("fixture file")
	}
	uid := uint32(os.Getuid())
	if _, err := readProtected(path, uid, dir); err != nil {
		t.Fatal("owned private fixture read")
	}
	if err := os.Chmod(dir, 0777); err != nil {
		t.Fatal("fixture chmod")
	}
	if _, err := readProtected(path, uid, dir); !errors.Is(err, ErrSSH) {
		t.Fatal("writable parent accepted")
	}
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal("fixture chmod restore")
	}
	alias := filepath.Join(dir, "alias")
	if err := os.Symlink(path, alias); err != nil {
		t.Fatal("fixture symlink")
	}
	if _, err := readProtected(alias, uid, dir); !errors.Is(err, ErrSSH) {
		t.Fatal("symlink accepted")
	}
	// Root trust never silently falls back to this user-owned local fixture.
	if uid != 0 {
		if _, err := ReadRootPolicy(path, digest(raw)); !errors.Is(err, ErrSSH) {
			t.Fatal("root trust downgraded")
		}
	}
	for _, bad := range []string{`{"x":1,"x":2}`, `{"x":{"y":1,"y":2}}`, `{"x":1} {"x":2}`} {
		var v any
		if decodeJSON([]byte(bad), &v) == nil {
			t.Fatal("duplicate or trailing JSON accepted")
		}
	}
}

func TestFullWorkflowPaginationAndTypedNilClient(t *testing.T) {
	p := fixturePolicy()
	for id := int64(7000); id < 7090; id++ {
		p.WorkflowIDs = append(p.WorkflowIDs, id)
	}
	token, keys := fixtureToken(t, p, nil)
	client := &fixtureClient{p: p, keys: keys}
	permit, err := AuthorizeProbe(context.Background(), client, p, p.Command(), p.SSHKeyFingerprint, token, p.LoginUID, time.Now())
	if err != nil || permit.Receipt().WorkflowCount != 111 {
		t.Fatal("complete multi-page workflow catalog rejected", err)
	}
	var nilClient *http.Client
	if _, err = AuthorizeProbe(context.Background(), nilClient, p, p.Command(), p.SSHKeyFingerprint, token, p.LoginUID, time.Now()); !errors.Is(err, ErrRemote) {
		t.Fatal("typed nil client did not fail closed")
	}
	//nolint:staticcheck // SA1012: This negative case intentionally supplies a nil context to require fail-closed host handling.
	if _, err = AuthorizeProbe(nil, client, p, p.Command(), p.SSHKeyFingerprint, token, p.LoginUID, time.Now()); !errors.Is(err, ErrPolicy) {
		t.Fatal("nil context did not fail closed")
	}
}
func TestEffectiveSSHDNeedsActualCompleteRestrictionProjection(t *testing.T) {
	rootPath := "/etc/qs-fence/authorized_keys"
	rows := []string{"authenticationmethods publickey", "authorizedkeysfile " + rootPath, "authorizedkeyscommand none", "trustedusercakeys none", "passwordauthentication no", "kbdinteractiveauthentication no", "hostbasedauthentication no", "gssapiauthentication no", "permituserenvironment no", "permituserrc no", "disableforwarding yes", "permittty no", "forcecommand none", "pubkeyauthentication yes", "strictmodes yes", "acceptenv LANG LC_*"}
	raw := []byte(strings.Join(rows, "\n") + "\n")
	if VerifyEffectiveSSHD(raw, rootPath) != nil {
		t.Fatal("restricted actual projection rejected")
	}
	for _, unsafe := range []string{"disableforwarding no", "passwordauthentication yes", "authorizedkeysfile .ssh/authorized_keys", "trustedusercakeys /etc/other-ca", "forcecommand internal-sftp", "acceptenv *"} {
		key := strings.SplitN(unsafe, " ", 2)[0]
		copyRows := append([]string(nil), rows...)
		for i, line := range copyRows {
			if strings.HasPrefix(line, key+" ") {
				copyRows[i] = unsafe
			}
		}
		if VerifyEffectiveSSHD([]byte(strings.Join(copyRows, "\n")), rootPath) == nil {
			t.Fatal("effective auth bypass accepted")
		}
	}
	if VerifyEffectiveSSHD(append(raw, []byte("strictmodes yes\n")...), rootPath) == nil {
		t.Fatal("ambiguous effective settings accepted")
	}
}
