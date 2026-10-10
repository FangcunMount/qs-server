package main

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func serviceTemplateTestRaw(t *testing.T) ([]byte, lifecycleServiceSessionRequest) {
	t.Helper()
	before := sourceSHA
	sourceSHA = strings.Repeat("a", 40)
	t.Cleanup(func() { sourceSHA = before })
	r := lifecycleServiceSessionRequest{FormatVersion: 1, Kind: "qs_root_service_session", ToolSourceSHA: sourceSHA,
		ToolBinarySHA256: strings.Repeat("b", 64), OriginalSourceSHA: strings.Repeat("c", 40), OperationID: "17-1",
		ManifestSHA256: strings.Repeat("d", 64), OriginalRunID: "16-1", DescriptorSHA256: strings.Repeat("e", 64)}
	raw, e := json.Marshal(r)
	if e != nil {
		t.Fatal(e)
	}
	return append(raw, '\n'), r
}

func TestAssignedServiceRunPreservesImmutableOriginalBindings(t *testing.T) {
	raw, original := serviceTemplateTestRaw(t)
	frozen := string(raw)
	for _, run := range []string{"37913340196-1", "37913340196-2"} {
		t.Run(run, func(t *testing.T) {
			derived, r, e := deriveLifecycleServiceTemplate(raw, digestRaw(raw), original.OperationID, run)
			if e != nil || r.ActualRunID != run {
				t.Fatal("new real caller run was not derived")
			}
			var actual lifecycleServiceSessionRequest
			if json.Unmarshal(derived, &actual) != nil || actual != r {
				t.Fatal("derived bytes disagree with bound request")
			}
			actual.ActualRunID = ""
			if actual != original || string(raw) != frozen {
				t.Fatal("derivation changed an original approval or template")
			}
			expected := filepath.Join(lifecycleServicesRoot(original.OperationID, "server-d"), "service-invocations", run)
			if lifecycleServiceInvocationRoot(original.OperationID, run) != expected {
				t.Fatal("per-run registry left fixed operation root")
			}
		})
	}
}

func TestAssignedServiceRunRefusesNullUnknownDuplicateAndChangedApproval(t *testing.T) {
	raw, original := serviceTemplateTestRaw(t)
	cases := map[string]string{
		"null_run":          strings.Replace(string(raw), `"actual_run_id":""`, `"actual_run_id":null`, 1),
		"already_bound_run": strings.Replace(string(raw), `"actual_run_id":""`, `"actual_run_id":"18-1"`, 1),
		"duplicate_run":     strings.Replace(string(raw), `"actual_run_id":""`, `"actual_run_id":"","actual_run_id":""`, 1),
		"case_alias":        strings.Replace(string(raw), `"actual_run_id"`, `"Actual_Run_Id"`, 1),
		"missing_run":       strings.Replace(string(raw), `,"actual_run_id":""`, "", 1),
		"permission":        strings.Replace(string(raw), `{`, `{"whole_writer_fence":true,`, 1),
		"window":            strings.Replace(string(raw), `{`, `{"window_complete":true,`, 1),
		"source":            strings.Replace(string(raw), sourceSHA, strings.Repeat("f", 40), 1),
		"operation":         strings.Replace(string(raw), original.OperationID, "19-1", 1),
		"null_original":     strings.Replace(string(raw), `"original_run_id":"16-1"`, `"original_run_id":null`, 1),
	}
	for name, value := range cases {
		t.Run(name, func(t *testing.T) {
			if _, _, e := deriveLifecycleServiceTemplate([]byte(value), digestRaw([]byte(value)), original.OperationID, "20-1"); e == nil {
				t.Fatal("ambiguous or changed template accepted")
			}
		})
	}
	for name, values := range map[string][3]string{
		"hash":            {strings.Repeat("f", 64), original.OperationID, "20-1"},
		"operation_input": {digestRaw(raw), "21-1", "20-1"},
		"run_input":       {digestRaw(raw), original.OperationID, ""},
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, e := deriveLifecycleServiceTemplate(raw, values[0], values[1], values[2]); e == nil {
				t.Fatal("unbound caller input accepted")
			}
		})
	}
}

func serviceChannelTemplateTestRequest(t *testing.T) (lifecycleRequest, lifecycleServiceSSHChannel) {
	t.Helper()
	_, r := serviceTemplateTestRaw(t)
	q := lifecycleRequest{ToolSourceSHA: r.ToolSourceSHA, OriginalSourceSHA: r.OriginalSourceSHA, OperationID: r.OperationID,
		ManifestSHA256: r.ManifestSHA256, ActualRunID: "20-1"}
	q.Recovery.OriginalRunID = r.OriginalRunID
	c := lifecycleServiceSSHChannel{FormatVersion: 1, Kind: "qs_existing_pinned_service_ssh_channel_template",
		ToolSourceSHA: q.ToolSourceSHA, OriginalSourceSHA: q.OriginalSourceSHA, OperationID: q.OperationID,
		ManifestSHA256: q.ManifestSHA256, OriginalRunID: r.OriginalRunID, SSHExecutableSHA256: strings.Repeat("a", 64),
		Host: "server-d.example", Port: 22, User: "qs", IdentitySHA256: strings.Repeat("b", 64), KnownHostsSHA256: strings.Repeat("c", 64),
		HostKeyFingerprint: "SHA256:approved-native-key", RemoteRequestSHA256: strings.Repeat("d", 64), RemoteDescriptorSHA256: strings.Repeat("e", 64)}
	return q, c
}

func TestPinnedServiceTemplateChannelUsesActualRunAndOriginalHashes(t *testing.T) {
	r, c := serviceChannelTemplateTestRequest(t)
	if !c.bindingMatches(r) {
		t.Fatal("independent fixed channel did not bind")
	}
	original := c
	for _, recovery := range []bool{false, true} {
		args := lifecycleServiceSSHArgs(r, c, recovery)
		command := args[len(args)-1]
		mode := "host-services-d-template"
		if recovery {
			mode = "host-services-d-recovery-template"
		}
		for _, part := range []string{"/usr/bin/sudo -n -- ", "--mode " + mode, "service-session-template.json", "--request-hash " + c.RemoteRequestSHA256, "--run-id " + r.ActualRunID, "--operation-id " + r.OperationID} {
			if !strings.Contains(command, part) {
				t.Fatal("fixed template route omitted actual caller binding")
			}
		}
		if c != original || strings.Contains(command, "--source-sha ") || strings.Contains(command, "fence=") {
			t.Fatal("route changed source approval or fabricated permission")
		}
	}
	old := c
	old.Kind = "qs_existing_pinned_service_ssh_channel"
	old.ActualRunID = r.ActualRunID
	if !old.bindingMatches(r) || strings.Contains(strings.Join(lifecycleServiceSSHArgs(r, old, false), " "), "-template") {
		t.Fatal("existing fixed-run channel changed")
	}
	old.ActualRunID = ""
	if old.bindingMatches(r) {
		t.Fatal("ordinary channel accepted blank current run")
	}
	c.ActualRunID = r.ActualRunID
	if c.bindingMatches(r) {
		t.Fatal("template channel silently accepted an already assigned run")
	}
}

func TestPinnedServiceTemplateChannelNullAssignedRunCannotMeanEmpty(t *testing.T) {
	_, c := serviceChannelTemplateTestRequest(t)
	raw, e := json.Marshal(c)
	if e != nil || validateLifecycleServiceChannelAssignedRun(raw, c) != nil {
		t.Fatal("exact independent channel rejected")
	}
	for name, value := range map[string]string{
		"null":      strings.Replace(string(raw), `"actual_run_id":""`, `"actual_run_id":null`, 1),
		"missing":   strings.Replace(string(raw), `,"actual_run_id":""`, "", 1),
		"extra":     strings.Replace(string(raw), `{`, `{"execution_allowed":true,`, 1),
		"duplicate": strings.Replace(string(raw), `{`, `{"actual_run_id":"",`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			if validateLifecycleServiceChannelAssignedRun([]byte(value), c) == nil {
				t.Fatal("invalid empty-run form accepted")
			}
		})
	}
}

func TestServiceTemplatePreflightDoesNotCreateAuthorityOrCallEffects(t *testing.T) {
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	for _, ctx := range []context.Context{nil, canceled, context.Background()} {
		path, hash, e := stageLifecycleServiceTemplate(ctx, filepath.Join(t.TempDir(), "service-session-template.json"), strings.Repeat("a", 64), "17-1", "20-1", false)
		if e == nil || path != "" || hash != "" {
			t.Fatal("unapproved local namespace produced a service invocation")
		}
	}
	if lifecycleEffectsPreflight(context.Background()) == nil {
		t.Fatal("metadata dispatcher enabled missing production effects")
	}
}
