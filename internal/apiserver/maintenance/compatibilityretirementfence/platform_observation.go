package compatibilityretirementfence

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/FangcunMount/qs-server/pkg/version"
)

var ErrPlatformObservation = errors.New("retirement_platform_quarantine_unproven")

// RunnerWorkflowScope supplies expected identities only. The current Action
// run and job are actually read; no saved receipt or completion is an input.
type RunnerWorkflowScope struct {
	FormatVersion       int     `json:"format_version"`
	Kind                string  `json:"kind"`
	DispatcherSourceSHA string  `json:"dispatcher_source_sha"`
	ToolSourceSHA       string  `json:"tool_source_sha"`
	OriginalSourceSHA   string  `json:"original_source_sha"`
	OperationID         string  `json:"operation_id"`
	OriginalRunID       string  `json:"original_run_id"`
	ManifestSHA256      string  `json:"manifest_sha256"`
	RepositoryID        string  `json:"repository_id"`
	OwnerID             string  `json:"owner_id"`
	ActorID             string  `json:"actor_id"`
	WorkflowID          int64   `json:"workflow_id"`
	WorkflowIDs         []int64 `json:"workflow_ids"`
	JobName             string  `json:"job_name"`
	RunnerID            int64   `json:"runner_id"`
}

func (s RunnerWorkflowScope) valid() bool {
	if s.FormatVersion != 1 || s.Kind != "approved_runner_workflow_quarantine_scope" ||
		!sha40.MatchString(s.DispatcherSourceSHA) || !sha40.MatchString(s.ToolSourceSHA) || !sha40.MatchString(s.OriginalSourceSHA) ||
		!operationID.MatchString(s.OperationID) || !operationID.MatchString(s.OriginalRunID) || !sha64.MatchString(s.ManifestSHA256) ||
		!decimal.MatchString(s.RepositoryID) || !decimal.MatchString(s.OwnerID) || !decimal.MatchString(s.ActorID) ||
		s.WorkflowID <= 0 || s.RunnerID <= 0 || len(s.WorkflowIDs) < 1 || len(s.WorkflowIDs) > 1000 || len(s.JobName) < 1 || len(s.JobName) > 200 {
		return false
	}
	for _, r := range s.JobName {
		if r != ' ' && r != '(' && r != ')' && r != '_' && r != '.' && r != '-' && (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') && (r < '0' || r > '9') {
			return false
		}
	}
	found := false
	for i, id := range s.WorkflowIDs {
		if id <= 0 || i > 0 && id <= s.WorkflowIDs[i-1] {
			return false
		}
		found = found || id == s.WorkflowID
	}
	return found
}

func decodeRunnerWorkflowScope(raw []byte) (RunnerWorkflowScope, error) {
	var s RunnerWorkflowScope
	if decodeJSON(raw, &s) != nil {
		return s, ErrPlatformObservation
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || len(fields) != 15 {
		return s, ErrPlatformObservation
	}
	for _, name := range []string{"format_version", "kind", "dispatcher_source_sha", "tool_source_sha", "original_source_sha", "operation_id", "original_run_id", "manifest_sha256", "repository_id", "owner_id", "actor_id", "workflow_id", "workflow_ids", "job_name", "runner_id"} {
		if _, ok := fields[name]; !ok || bytes.Equal(bytes.TrimSpace(fields[name]), []byte("null")) {
			return s, ErrPlatformObservation
		}
	}
	if !s.valid() {
		return s, ErrPlatformObservation
	}
	return s, nil
}

// PlatformObservation proves only two current native API reads. It does not
// own the runner's mutation lease, prove old credential paths are blocked, or
// provide a whole-writer fence/DDL capability.
type PlatformObservation struct {
	self    *PlatformObservation
	window  *MaintenanceWindow
	start   string
	binding WindowBinding
	scope   RunnerWorkflowScope
	run     string
	digest  string
	job     int64
	count   int
}

func (*PlatformObservation) MarshalJSON() ([]byte, error) { return nil, ErrPlatformObservation }
func (*PlatformObservation) String() string {
	return "opaque native current platform observation; not a whole-writer fence"
}

type PlatformObservationSummary struct {
	SnapshotSHA256         string `json:"snapshot_sha256"`
	WorkflowCount          int    `json:"workflow_count"`
	JobID                  int64  `json:"job_id"`
	WholeWriterFenceProven bool   `json:"whole_writer_fence_proven"`
	DropReady              bool   `json:"drop_ready"`
}

func (o *PlatformObservation) Summary() PlatformObservationSummary {
	if o == nil || o.self != o {
		return PlatformObservationSummary{}
	}
	return PlatformObservationSummary{SnapshotSHA256: o.digest, WorkflowCount: o.count, JobID: o.job}
}

func (o *PlatformObservation) ValidateOriginalWindow(ctx context.Context, w *MaintenanceWindow, original WindowBinding) error {
	if o == nil || o.self != o || w == nil || o.window != w || o.binding != original || ctx == nil || ctx.Err() != nil {
		return ErrPlatformObservation
	}
	d, e := w.Diagnostic(ctx)
	if e != nil || d.Binding != o.binding || d.StartSHA256 != o.start || !d.DirectoryLeaseHeld || d.RemainingMilliseconds <= 0 {
		return ErrPlatformObservation
	}
	return nil // Not freshness; consumers must call the actual producer again.
}

type platformReadTransport struct {
	token []byte
	inner http.RoundTripper
}

func (t *platformReadTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil || req.Method != http.MethodGet || req.URL.Scheme != "https" || req.URL.Host != "api.github.com" || req.URL.User != nil || req.URL.Fragment != "" || req.URL.Path != "/repos/"+Repository && !strings.HasPrefix(req.URL.Path, "/repos/"+Repository+"/") {
		return nil, ErrPlatformObservation
	}
	copyReq := req.Clone(req.Context())
	copyReq.Header.Set("Authorization", "Bearer "+string(t.token))
	return t.inner.RoundTrip(copyReq)
}

// ObservePlatformQuarantine uses its own fixed native GET-only HTTPS transport.
// Token bytes are borrowed only for this call, copied to memory and cleared;
// no host/runner authentication or raw credential is returned or persisted.
func ObservePlatformQuarantine(ctx context.Context, w *MaintenanceWindow, scopePath, scopeSHA256, actualRun, dispatcherSourceSHA string, ephemeralToken []byte) (*PlatformObservation, error) {
	if runtime.GOOS != "linux" || os.Getuid() != 0 || os.Geteuid() != 0 || ctx == nil || ctx.Err() != nil || w == nil || !sha64.MatchString(scopeSHA256) || !operationID.MatchString(actualRun) || !sha40.MatchString(dispatcherSourceSHA) || len(ephemeralToken) < 1 || len(ephemeralToken) > 8192 {
		return nil, ErrPlatformObservation
	}
	for _, c := range ephemeralToken {
		if c < 33 || c > 126 {
			return nil, ErrPlatformObservation
		}
	}
	raw, e := readProtected(scopePath, 0, "/")
	if e != nil || digest(raw) != scopeSHA256 {
		return nil, ErrPlatformObservation
	}
	s, e := decodeRunnerWorkflowScope(raw)
	if e != nil || s.ToolSourceSHA != version.GitCommit || s.DispatcherSourceSHA != dispatcherSourceSHA || scopePath != "/opt/qs-server/qs-apiserver/compatibility-retirement/"+s.OperationID+"/approved-workflow-scope.json" {
		return nil, ErrPlatformObservation
	}
	d, e := w.Diagnostic(ctx)
	if e != nil || d.Binding != (WindowBinding{TargetSHA256: MaintenanceWindowTargetSHA256(), SourceSHA: s.OriginalSourceSHA, OperationID: s.OperationID, ManifestSHA256: s.ManifestSHA256, OriginalRunID: s.OriginalRunID}) || !d.DirectoryLeaseHeld || d.RemainingMilliseconds <= 0 {
		return nil, ErrPlatformObservation
	}
	var q context.Context
	var cancel context.CancelFunc
	if d.RecoverySHA256 == "" {
		q, cancel, e = w.ForwardContext(ctx)
	} else {
		q, cancel, e = w.RecoveryContext(ctx)
	}
	if e != nil {
		return nil, e
	}
	defer cancel()
	transport := &http.Transport{Proxy: nil, TLSHandshakeTimeout: 10 * time.Second, ForceAttemptHTTP2: true, MaxIdleConns: 4, IdleConnTimeout: 30 * time.Second}
	defer transport.CloseIdleConnections()
	auth := &platformReadTransport{token: bytes.Clone(ephemeralToken), inner: transport}
	defer clear(auth.token)
	client := &http.Client{Transport: auth, Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	snapshot, e := readPlatformQuarantinePair(q, client, s, actualRun)
	if e != nil || q.Err() != nil {
		return nil, ErrPlatformObservation
	}
	if again, e := readProtected(scopePath, 0, "/"); e != nil || !bytes.Equal(again, raw) {
		return nil, ErrPlatformObservation
	}
	o := &PlatformObservation{window: w, binding: d.Binding, start: d.StartSHA256, scope: s, run: actualRun, digest: snapshot.digest, job: snapshot.job, count: snapshot.count}
	o.self = o
	if e = o.ValidateOriginalWindow(q, w, d.Binding); e != nil {
		return nil, e
	}
	return o, nil
}

type platformSnapshot struct {
	digest string
	job    int64
	count  int
}

func getPlatformJSON(ctx context.Context, client HTTPDoer, address string, dst any) ([]byte, error) {
	if ctx == nil || ctx.Err() != nil || client == nil || nilDoer(client) {
		return nil, ErrPlatformObservation
	}
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if e != nil {
		return nil, ErrPlatformObservation
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	response, e := client.Do(req)
	if e != nil || response == nil || response.Body == nil {
		return nil, ErrPlatformObservation
	}
	raw, readErr := io.ReadAll(io.LimitReader(response.Body, MaxBodyBytes+1))
	closeErr := response.Body.Close()
	if readErr != nil || closeErr != nil || len(raw) > MaxBodyBytes || response.StatusCode != http.StatusOK || response.Request == nil || response.Request.URL.String() != address || decodeJSON(raw, dst) != nil || ctx.Err() != nil {
		return nil, ErrPlatformObservation
	}
	return raw, nil
}

// Private protocol seam, never a public native factory or a whole-fence token.
func readPlatformQuarantinePair(ctx context.Context, client HTTPDoer, s RunnerWorkflowScope, run string) (platformSnapshot, error) {
	a, e := readPlatformQuarantine(ctx, client, s, run)
	if e != nil {
		return a, e
	}
	b, e := readPlatformQuarantine(ctx, client, s, run)
	if e != nil || a != b {
		return a, ErrPlatformObservation
	}
	return a, nil
}

func readPlatformQuarantine(ctx context.Context, client HTTPDoer, s RunnerWorkflowScope, run string) (platformSnapshot, error) {
	var result platformSnapshot
	if !s.valid() || !operationID.MatchString(run) {
		return result, ErrPlatformObservation
	}
	runParts := strings.Split(run, "-")
	rid, attempt := runParts[0], runParts[1]
	base := APIBase + "/repos/" + Repository
	var hashes []string
	read := func(path string, dst any) error {
		raw, e := getPlatformJSON(ctx, client, base+path, dst)
		if e == nil {
			hashes = append(hashes, path+" "+digest(raw))
		}
		return e
	}
	var repo githubRepository
	if read("", &repo) != nil || canonicalInteger(repo.ID) != s.RepositoryID || canonicalInteger(repo.Owner.ID) != s.OwnerID || repo.FullName != Repository {
		return result, ErrPlatformObservation
	}
	var main struct {
		SHA string `json:"sha"`
	}
	if read("/commits/main", &main) != nil || main.SHA != s.DispatcherSourceSHA {
		return result, ErrPlatformObservation
	}
	var current githubRun
	if read("/actions/runs/"+rid+"/attempts/"+attempt, &current) != nil || canonicalInteger(current.ID) != rid || canonicalInteger(current.RunAttempt) != attempt || current.WorkflowID != s.WorkflowID || current.HeadSHA != s.DispatcherSourceSHA || current.HeadBranch != "main" || current.Event != "workflow_dispatch" || current.Status != "in_progress" || current.Path != WorkflowPath || canonicalInteger(current.Actor.ID) != s.ActorID || canonicalInteger(current.TriggeringActor.ID) != s.ActorID || canonicalInteger(current.Repository.ID) != s.RepositoryID || current.Repository.FullName != Repository || canonicalInteger(current.HeadRepository.ID) != s.RepositoryID || current.HeadRepository.FullName != Repository {
		return result, ErrPlatformObservation
	}
	workflows := map[int64]githubWorkflow{}
	if e := readPlatformList(read, "/actions/workflows", "workflows", func(raw json.RawMessage) (int64, error) {
		var row githubWorkflow
		if decodeJSON(raw, &row) != nil || row.ID <= 0 || row.Path == "" || workflows[row.ID].ID != 0 {
			return 0, ErrPlatformObservation
		}
		if row.State != "active" && row.State != "disabled_manually" && row.State != "disabled_inactivity" && row.State != "disabled_fork" {
			return 0, ErrPlatformObservation
		}
		if row.ID == s.WorkflowID {
			if row.Path != WorkflowPath || row.State != "active" {
				return 0, ErrPlatformObservation
			}
		} else if row.State == "active" {
			return 0, ErrPlatformObservation
		}
		workflows[row.ID] = row
		return row.ID, nil
	}); e != nil || len(workflows) != len(s.WorkflowIDs) {
		return result, ErrPlatformObservation
	}
	for _, id := range s.WorkflowIDs {
		if workflows[id].ID != id {
			return result, ErrPlatformObservation
		}
	}
	queueRows := 0
	for _, status := range []string{"queued", "in_progress", "waiting", "pending", "requested"} {
		if e := readPlatformList(read, "/actions/runs?status="+status, "workflow_runs", func(raw json.RawMessage) (int64, error) {
			var row githubRun
			if decodeJSON(raw, &row) != nil || canonicalInteger(row.ID) != rid || canonicalInteger(row.RunAttempt) != attempt || row.WorkflowID != s.WorkflowID || status != "in_progress" || row.Status != status || row.HeadSHA != s.DispatcherSourceSHA || canonicalInteger(row.Repository.ID) != s.RepositoryID || row.Repository.FullName != Repository {
				return 0, ErrPlatformObservation
			}
			queueRows++
			return row.ID, nil
		}); e != nil {
			return result, e
		}
	}
	if queueRows != 1 {
		return result, ErrPlatformObservation
	}
	if e := readPlatformList(read, "/actions/runs/"+rid+"/attempts/"+attempt+"/jobs", "jobs", func(raw json.RawMessage) (int64, error) {
		var row struct {
			githubJob
			Name string `json:"name"`
		}
		if decodeJSON(raw, &row) != nil || row.ID <= 0 || canonicalInteger(row.RunID) != rid || row.HeadSHA != s.DispatcherSourceSHA {
			return 0, ErrPlatformObservation
		}
		if row.Name == s.JobName {
			if result.job != 0 || row.Status != "in_progress" || row.RunnerID != s.RunnerID {
				return 0, ErrPlatformObservation
			}
			result.job = row.ID
		} else if row.Status != "completed" {
			return 0, ErrPlatformObservation
		}
		return row.ID, nil
	}); e != nil || result.job == 0 {
		return result, ErrPlatformObservation
	}
	sort.Strings(hashes)
	result.digest, result.count = digest([]byte(strings.Join(hashes, "\n"))), len(workflows)
	return result, nil
}

func readPlatformList(read func(string, any) error, path, rowKey string, consume func(json.RawMessage) (int64, error)) error {
	total := -1
	seen := map[int64]bool{}
	separator := "?"
	if strings.Contains(path, "?") {
		separator = "&"
	}
	for page := 1; page <= 11; page++ {
		var list map[string]json.RawMessage
		if read(fmt.Sprintf("%s%sper_page=100&page=%d", path, separator, page), &list) != nil {
			return ErrPlatformObservation
		}
		var count int
		var rows []json.RawMessage
		if bytes.Equal(bytes.TrimSpace(list["total_count"]), []byte("null")) || bytes.Equal(bytes.TrimSpace(list[rowKey]), []byte("null")) || json.Unmarshal(list["total_count"], &count) != nil || json.Unmarshal(list[rowKey], &rows) != nil || count < 0 || count > 1000 || len(rows) > 100 {
			return ErrPlatformObservation
		}
		if total < 0 {
			total = count
		}
		if total != count {
			return ErrPlatformObservation
		}
		for _, row := range rows {
			id, e := consume(row)
			if e != nil || id <= 0 || seen[id] {
				return ErrPlatformObservation
			}
			seen[id] = true
		}
		if len(rows) < 100 {
			if len(seen) != total {
				return ErrPlatformObservation
			}
			return nil
		}
	}
	return ErrPlatformObservation
}
