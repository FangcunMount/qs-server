package compatibilityretirementfence

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"strings"
	"testing"
)

// A private protocol adapter exercises schema/pagination/order only. This is
// not the native TLS factory, GitHub's identity, root, or a writer-fence proof.
type platformProtocolFixture struct {
	base      *fixtureClient
	mutation  string
	workflows []githubWorkflow
	runs      []githubRun
}

func platformScopeFixture() (RunnerWorkflowScope, *platformProtocolFixture) {
	p := fixturePolicy()
	ids := append([]int64(nil), p.WorkflowIDs...)
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	s := RunnerWorkflowScope{FormatVersion: 1, Kind: "approved_runner_workflow_quarantine_scope", DispatcherSourceSHA: p.SourceSHA,
		ToolSourceSHA: strings.Repeat("b", 40), OriginalSourceSHA: strings.Repeat("c", 40), OperationID: p.OperationID,
		OriginalRunID: "901-1", ManifestSHA256: strings.Repeat("d", 64), RepositoryID: p.RepositoryID, OwnerID: p.OwnerID,
		ActorID: p.ActorID, WorkflowID: p.WorkflowID, WorkflowIDs: ids, JobName: "Exact controlled fixture", RunnerID: p.RunnerID}
	return s, &platformProtocolFixture{base: &fixtureClient{p: p}}
}

func platformManagedAnalysisFixture() (RunnerWorkflowScope, *platformProtocolFixture) {
	s, f := platformScopeFixture()
	// Exact identities observed in the repository's complete workflow catalog.
	// Keep this fixture independent of the producer's classification function.
	f.workflows = []githubWorkflow{
		{ID: 214640618, Path: "dynamic/dependabot/update-graph", Name: "Dependency Graph", State: "active"},
		{ID: 264440496, Path: "dynamic/github-code-scanning/codeql", Name: "CodeQL", State: "active"},
		{ID: 343442498, Path: "dynamic/github-code-quality/codeql", Name: "CodeQL - Code Quality", State: "active"},
	}
	for _, row := range f.workflows {
		s.WorkflowIDs = append(s.WorkflowIDs, row.ID)
	}
	sort.Slice(s.WorkflowIDs, func(i, j int) bool { return s.WorkflowIDs[i] < s.WorkflowIDs[j] })
	return s, f
}

func TestApprovedWorkflowMaterialBindingUsesOriginalScope(t *testing.T) {
	s, _ := platformScopeFixture()
	raw, e := json.Marshal(s)
	if e != nil {
		t.Fatal(e)
	}
	b := WindowBinding{TargetSHA256: MaintenanceWindowTargetSHA256(), SourceSHA: s.OriginalSourceSHA, OperationID: s.OperationID, ManifestSHA256: s.ManifestSHA256, OriginalRunID: s.OriginalRunID}
	if ValidateRunnerWorkflowScopeBinding(raw, b, s.ToolSourceSHA) != nil {
		t.Fatal("original approved material metadata rejected")
	}
	for _, changed := range []WindowBinding{{}, {TargetSHA256: b.TargetSHA256, SourceSHA: b.SourceSHA, OperationID: "999-1", ManifestSHA256: b.ManifestSHA256, OriginalRunID: b.OriginalRunID}} {
		if ValidateRunnerWorkflowScopeBinding(raw, changed, s.ToolSourceSHA) == nil {
			t.Fatal("another operation's workflow material was adopted")
		}
	}
	if ValidateRunnerWorkflowScopeBinding(raw, b, strings.Repeat("f", 40)) == nil {
		t.Fatal("another tool's workflow material was adopted")
	}
	s.WorkflowIDs = []int64{s.WorkflowID, s.WorkflowID}
	raw, _ = json.Marshal(s)
	if ValidateRunnerWorkflowScopeBinding(raw, b, s.ToolSourceSHA) == nil {
		t.Fatal("invalid approved workflow scope was adopted")
	}
}

func (f *platformProtocolFixture) Do(req *http.Request) (*http.Response, error) {
	response, e := f.base.Do(req)
	if e != nil || response == nil || response.Body == nil {
		return response, e
	}
	raw, e := io.ReadAll(response.Body)
	closeErr := response.Body.Close()
	if e != nil || closeErr != nil {
		return nil, ErrPlatformObservation
	}
	var v map[string]any
	if json.Unmarshal(raw, &v) != nil {
		return nil, ErrPlatformObservation
	}
	if strings.HasSuffix(req.URL.Path, "/actions/workflows") {
		for _, item := range v["workflows"].([]any) {
			row := item.(map[string]any)
			if int64(row["id"].(float64)) != f.base.p.WorkflowID {
				row["state"] = "disabled_manually"
				switch f.mutation {
				case "open_workflow":
					row["state"] = "active"
				case "unknown_state":
					row["state"] = "unrecognized"
				}
			}
		}
		rows := v["workflows"].([]any)
		for _, row := range f.workflows {
			rows = append(rows, row)
		}
		v["workflows"] = rows
		v["total_count"] = v["total_count"].(float64) + float64(len(f.workflows))
	}
	if strings.HasSuffix(req.URL.Path, "/actions/runs") {
		rows := v["workflow_runs"].([]any)
		added := 0
		for _, row := range f.runs {
			if row.Status == req.URL.Query().Get("status") {
				rows = append(rows, row)
				added++
			}
		}
		v["workflow_runs"] = rows
		v["total_count"] = v["total_count"].(float64) + float64(added)
	}
	if strings.HasSuffix(req.URL.Path, "/jobs") {
		for _, item := range v["jobs"].([]any) {
			row := item.(map[string]any)
			if row["check_run_url"] == APIBase+"/repos/"+Repository+"/check-runs/"+f.base.p.CheckRunID {
				row["name"] = "Exact controlled fixture"
			} else {
				row["name"] = "Finished fixture job"
			}
		}
	}
	if f.mutation == "null_rows" && strings.HasSuffix(req.URL.Path, "/actions/runs") {
		v["workflow_runs"], v["total_count"] = nil, 0
	}
	if f.mutation == "changing_snapshot" && f.base.snapshots > 1 && strings.HasSuffix(req.URL.Path, "/actions/workflows") {
		v["unrelated_provider_value"] = "changed"
	}
	raw, e = json.Marshal(v)
	if e != nil {
		return nil, e
	}
	response.Body = io.NopCloser(bytes.NewReader(raw))
	return response, nil
}

func TestPlatformQuarantineProtocolClosedTwoSnapshots(t *testing.T) {
	s, f := platformScopeFixture()
	v, e := readPlatformQuarantinePair(t.Context(), f, s, "900-1")
	if e != nil || !sha64.MatchString(v.digest) || v.count != len(s.WorkflowIDs) || v.job <= 0 || f.base.snapshots != 2 {
		t.Fatal("closed fixed current-run protocol did not reach two complete snapshots", e)
	}
	for _, name := range []string{"open_workflow", "unknown_state", "null_rows", "changing_snapshot"} {
		t.Run(name, func(t *testing.T) {
			s, f := platformScopeFixture()
			f.mutation = name
			if _, e := readPlatformQuarantinePair(t.Context(), f, s, "900-1"); e == nil {
				t.Fatal("invalid current platform view accepted")
			}
		})
	}
}

func TestPlatformQuarantineProtocolRefusedOldAndPending(t *testing.T) {
	for _, name := range []string{"queued", "waiting", "pending", "requested", "in_progress", "unknown_workflow", "catalog_incomplete", "main_advanced", "attempt_changed", "old_main_sha", "runner_changed"} {
		t.Run(name, func(t *testing.T) {
			s, f := platformScopeFixture()
			f.base.mutation = name
			if _, e := readPlatformQuarantinePair(t.Context(), f, s, "900-1"); e == nil {
				t.Fatal("old or pending platform writer entry accepted")
			}
		})
	}
}

func TestPlatformQuarantineManagedAnalysisCatalogOnly(t *testing.T) {
	s, f := platformManagedAnalysisFixture()
	v, e := readPlatformQuarantinePair(t.Context(), f, s, "900-1")
	if e != nil || v.count != len(s.WorkflowIDs) || !sha64.MatchString(v.digest) || v.job <= 0 || f.base.snapshots != 2 {
		t.Fatal("exact managed analysis catalog did not preserve complete two-snapshot observation", e)
	}
	for _, name := range []string{"missing_catalog_row", "missing_approved_id", "duplicate_catalog_row", "changing_snapshot", "open_workflow"} {
		t.Run(name, func(t *testing.T) {
			s, f := platformManagedAnalysisFixture()
			switch name {
			case "missing_catalog_row":
				f.workflows = f.workflows[1:]
			case "missing_approved_id":
				for i, id := range s.WorkflowIDs {
					if id == f.workflows[0].ID {
						s.WorkflowIDs = append(s.WorkflowIDs[:i], s.WorkflowIDs[i+1:]...)
						break
					}
				}
			case "duplicate_catalog_row":
				f.workflows = append(f.workflows, f.workflows[0])
			default:
				f.mutation = name
			}
			if _, e := readPlatformQuarantinePair(t.Context(), f, s, "900-1"); e == nil {
				t.Fatal("managed analysis classification weakened catalog or snapshot closure")
			}
		})
	}
}

func TestPlatformQuarantineManagedAnalysisRequiresExactIdentity(t *testing.T) {
	_, initial := platformManagedAnalysisFixture()
	for index, row := range initial.workflows {
		for _, field := range []string{"id", "path", "path_prefix", "name"} {
			t.Run(row.Name+"/"+field, func(t *testing.T) {
				s, f := platformManagedAnalysisFixture()
				switch field {
				case "id":
					f.workflows[index].ID++
					for i, id := range s.WorkflowIDs {
						if id == row.ID {
							s.WorkflowIDs[i] = f.workflows[index].ID
						}
					}
				case "path":
					f.workflows[index].Path = ".github/workflows/codeql.yml"
				case "path_prefix":
					f.workflows[index].Path += "/other"
				case "name":
					f.workflows[index].Name += " "
				}
				if _, e := readPlatformQuarantinePair(t.Context(), f, s, "900-1"); e == nil {
					t.Fatal("partial managed analysis identity accepted as an active catalog exception")
				}
			})
		}
	}
}

func TestPlatformQuarantineManagedAnalysisNonterminalRunsRefused(t *testing.T) {
	_, initial := platformManagedAnalysisFixture()
	for _, workflow := range initial.workflows {
		for _, status := range []string{"queued", "in_progress", "waiting", "pending", "requested"} {
			for _, origin := range []string{"current", "old_ref", "rerun"} {
				if origin != "current" && status != "in_progress" {
					continue
				}
				t.Run(workflow.Name+"/"+status+"/"+origin, func(t *testing.T) {
					s, f := platformManagedAnalysisFixture()
					repo := githubRepository{ID: 100, FullName: Repository}
					repo.Owner.ID = 200
					run := githubRun{ID: 902, RunAttempt: 1, WorkflowID: workflow.ID, HeadSHA: s.DispatcherSourceSHA,
						HeadBranch: "main", Event: "dynamic", Status: status, Path: workflow.Path, Repository: repo, HeadRepository: repo}
					switch origin {
					case "old_ref":
						run.HeadBranch, run.HeadSHA = "old", strings.Repeat("e", 40)
					case "rerun":
						run.RunAttempt = 2
					}
					f.runs = []githubRun{run}
					if _, e := readPlatformQuarantinePair(t.Context(), f, s, "900-1"); e == nil {
						t.Fatal("managed analysis active-state exception admitted a nonterminal run")
					}
				})
			}
		}
	}
}

func TestPlatformScopeExactAndNoImportedAuthority(t *testing.T) {
	s, _ := platformScopeFixture()
	raw, e := json.Marshal(s)
	if e != nil {
		t.Fatal(e)
	}
	if _, e := decodeRunnerWorkflowScope(raw); e != nil {
		t.Fatal(e)
	}
	for _, raw := range [][]byte{
		bytes.Replace(raw, []byte(`"kind"`), []byte(`"Kind"`), 1),
		bytes.Replace(raw, []byte(`"kind":`), []byte(`"drop_ready":true,"kind":`), 1),
		bytes.Replace(raw, []byte(`"kind":`), []byte(`"kind":null,"kind":`), 1),
		bytes.Replace(raw, []byte(`"runner_id":400`), []byte(`"runner_id":null`), 1),
	} {
		if _, e := decodeRunnerWorkflowScope(raw); e == nil {
			t.Fatal("loose or imported authority accepted")
		}
	}
	var zero PlatformObservation
	if _, e := json.Marshal(&zero); e == nil {
		t.Fatal("platform observation serialized")
	}
	if zero.Summary().DropReady || zero.Summary().WholeWriterFenceProven || zero.ValidateOriginalWindow(t.Context(), nil, WindowBinding{}) == nil {
		t.Fatal("zero platform observation gained authority")
	}
	if _, e := ObservePlatformQuarantine(context.Background(), nil, "", "", "", "", []byte("fixture")); e == nil {
		t.Fatal("public producer accepted missing native/root/window constraints")
	}
}

type rejectPlatformTransport struct{ calls int }

func (t *rejectPlatformTransport) RoundTrip(*http.Request) (*http.Response, error) {
	t.calls++
	return nil, ErrRemote
}

func TestPlatformNativeTransportGetOnlyCredentialBoundary(t *testing.T) {
	inner := &rejectPlatformTransport{}
	transport := &platformReadTransport{token: []byte("fixture-private-token"), inner: inner}
	for _, item := range [][2]string{{"PUT", APIBase + "/repos/" + Repository + "/actions/workflows/10/disable"}, {"GET", "https://other.invalid/repos/" + Repository}, {"GET", APIBase + "/repos/other/project"}} {
		req, e := http.NewRequest(item[0], item[1], nil)
		if e != nil {
			t.Fatal(e)
		}
		if _, e := transport.RoundTrip(req); e == nil {
			t.Fatal("credential or method boundary widened")
		}
	}
	if inner.calls != 0 {
		t.Fatal("invalid request reached underlying transport")
	}
}
