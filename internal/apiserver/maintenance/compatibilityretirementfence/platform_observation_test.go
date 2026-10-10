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
	base     *fixtureClient
	mutation string
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
