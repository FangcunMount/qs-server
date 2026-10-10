package main

import (
	"context"
	"encoding/json"
	"reflect"
	"runtime"
	"runtime/debug"
	"strings"
	"testing"

	backup "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementbackup"
	fence "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementfence"
	stop "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementstop"
	"github.com/FangcunMount/qs-server/internal/pkg/migration"
)

func inlineAPIExpectedFixture(t *testing.T) (lifecycleAPIInspection, lifecycleRequest) {
	t.Helper()
	raw := func(v any) json.RawMessage {
		p, e := json.Marshal(v)
		if e != nil {
			t.Fatal(e)
		}
		return p
	}
	v := lifecycleAPIInspection{ID: strings.Repeat("1", 64), Name: "/qs-apiserver", Image: "sha256:" + strings.Repeat("2", 64),
		Config: map[string]json.RawMessage{"Image": raw("original-image"), "Entrypoint": raw([]string{"/app/qs-apiserver"}), "Cmd": raw([]string{"--config", "/etc/qs-server/apiserver.yaml", "--migration.enabled=true"}),
			"Env": raw([]string{"DUMMY_CONFIG=kept"}), "Labels": raw(map[string]string{"com.docker.compose.project": "qs-server", "com.docker.compose.service": "apiserver"}), "WorkingDir": raw("/app"), "User": raw("1000"), "ExposedPorts": raw(map[string]any{"8081/tcp": map[string]any{}})},
		HostConfig: map[string]json.RawMessage{"NetworkMode": raw("qs-network"), "Binds": raw([]string{"/opt/qs-server/config:/etc/qs-server:ro", "/data/infra/qs-server-messaging/config:/etc/qs-messaging:ro"}), "RestartPolicy": raw(map[string]any{"Name": "unless-stopped", "MaximumRetryCount": 0}), "PortBindings": raw(map[string]any{"8081/tcp": []map[string]string{{"HostIp": "127.0.0.1", "HostPort": "8081"}}})}}
	v.NetworkSettings.Networks = map[string]json.RawMessage{"qs-network": raw(map[string]any{"Aliases": []string{"qs-apiserver"}, "IPAMConfig": nil, "IPAddress": "172.20.0.2", "EndpointID": "old-ephemeral"})}
	r := lifecycleRequest{ToolSourceSHA: strings.Repeat("a", 40), OriginalSourceSHA: strings.Repeat("b", 40), OperationID: "17-1", ActualRunID: "18-1",
		DeploymentControl: &lifecycleAPIDeploymentControl{BImageID: "sha256:" + strings.Repeat("3", 64), BProgramSHA256: strings.Repeat("4", 64), RollbackSourceSHA: strings.Repeat("c", 40), RollbackImageID: v.Image, RollbackProgramSHA256: strings.Repeat("5", 64), OriginalRuntimeSpecSHA256: digest(v.spec())}}
	return v, r
}

// These are pure expected-payload contracts, not real Docker/runtime/DB proof.
func TestInlineAPICreatePreservesCurrentConfigMQAndNetworks(t *testing.T) {
	original, r := inlineAPIExpectedFixture(t)
	frozen, _ := json.Marshal(original)
	for _, rollback := range []bool{false, true} {
		t.Run(map[bool]string{false: "b", true: "rollback"}[rollback], func(t *testing.T) {
			image := r.DeploymentControl.BImageID
			if rollback {
				image = r.DeploymentControl.RollbackImageID
			}
			body, e := lifecycleAPICreateBody(original, image, rollback, r)
			if e != nil {
				t.Fatal(e)
			}
			var payload map[string]json.RawMessage
			if json.Unmarshal(body, &payload) != nil {
				t.Fatal("bad payload")
			}
			for _, field := range []string{"Env", "Entrypoint", "WorkingDir", "User", "ExposedPorts"} {
				if !reflect.DeepEqual(payload[field], original.Config[field]) {
					t.Fatal("current configuration changed: " + field)
				}
			}
			var host map[string]json.RawMessage
			if json.Unmarshal(payload["HostConfig"], &host) != nil || !reflect.DeepEqual(host, original.HostConfig) {
				t.Fatal("MQ mounts or current HostConfig changed")
			}
			var cmd []string
			if json.Unmarshal(payload["Cmd"], &cmd) != nil {
				t.Fatal("bad command")
			}
			expectedFlag := "--migration.enabled=true"
			if rollback {
				expectedFlag = "--migration.enabled=false"
			}
			if !reflect.DeepEqual(cmd, []string{"--config", "/etc/qs-server/apiserver.yaml", expectedFlag}) {
				t.Fatal("migration mode not explicit")
			}
			var network struct {
				EndpointsConfig map[string]map[string]json.RawMessage
			}
			if json.Unmarshal(payload["NetworkingConfig"], &network) != nil || len(network.EndpointsConfig) != 1 || network.EndpointsConfig["qs-network"] == nil {
				t.Fatal("current network lost")
			}
			if _, ok := network.EndpointsConfig["qs-network"]["EndpointID"]; ok {
				t.Fatal("reused daemon-owned endpoint identity")
			}
			if _, ok := network.EndpointsConfig["qs-network"]["IPAddress"]; ok {
				t.Fatal("invented static IP from allocated address")
			}
		})
	}
	after, _ := json.Marshal(original)
	if !reflect.DeepEqual(frozen, after) {
		t.Fatal("payload derivation mutated original runtime facts")
	}
}

func TestInlineAPIRollbackFlagCannotCarryAmbiguousAutomaticMigration(t *testing.T) {
	for name, cmd := range map[string][]string{"equals": {"--migration.enabled=true", "--config", "x"}, "split": {"--migration.enabled", "true", "--config", "x"}, "existing_false": {"--config", "x", "--migration.enabled=false"}} {
		t.Run(name, func(t *testing.T) {
			actual, e := lifecycleAPINoMigrationCommand(cmd)
			if e != nil || !reflect.DeepEqual(actual, []string{"--config", "x", "--migration.enabled=false"}) {
				t.Fatal("automatic migration flag was retained")
			}
		})
	}
	for name, cmd := range map[string][]string{"missing": {"--migration.enabled"}, "other_value": {"--migration.enabled=unknown"}, "split_unknown": {"--migration.enabled", "unknown"}} {
		t.Run(name, func(t *testing.T) {
			if _, e := lifecycleAPINoMigrationCommand(cmd); e == nil {
				t.Fatal("ambiguous flag accepted")
			}
		})
	}
}
func TestInlineAPIExpectedImageFactsCannotSupplyNativeCapability(t *testing.T) {
	_, r := inlineAPIExpectedFixture(t)
	if !r.DeploymentControl.valid() {
		t.Fatal("valid expected binding rejected")
	}
	for name, change := range map[string]func(*lifecycleAPIDeploymentControl){"tag": func(v *lifecycleAPIDeploymentControl) { v.BImageID = "latest" }, "missing_program": func(v *lifecycleAPIDeploymentControl) { v.BProgramSHA256 = "" }, "source": func(v *lifecycleAPIDeploymentControl) { v.RollbackSourceSHA = "unknown" }, "config": func(v *lifecycleAPIDeploymentControl) { v.OriginalRuntimeSpecSHA256 = "" }} {
		t.Run(name, func(t *testing.T) {
			v := *r.DeploymentControl
			change(&v)
			if v.valid() {
				t.Fatal("unbound expectation accepted")
			}
		})
	}
	h := &lifecycleFixedHost{}
	for _, p := range []*migration.CompatibilityPairMigrationProof{nil, new(migration.CompatibilityPairMigrationProof)} {
		if h.DeployBInline(context.Background(), r, p, new(fence.MaintenanceWindow)) == nil {
			t.Fatal("expected image/zero proof enabled deployment")
		}
	}
	for _, p := range []*lifecycleRecoveryReadback{nil, {}, {direct: new(backup.TargetRecoveryVerification)}, {resumed: new(backup.TargetLifecycleResult)}} {
		if h.DeployRollbackInline(context.Background(), r, p, new(fence.MaintenanceWindow)) == nil {
			t.Fatal("zero recovery readback enabled rollback")
		}
		if p != nil && p.verify(context.Background(), backup.TargetRecoveryBorrowed{}, r, new(fence.MaintenanceWindow)) == nil {
			t.Fatal("empty borrowed handles became recovery proof")
		}
	}
	if lifecycleInlineMigrationBindingMatches(r, migration.CompatibilityPairMigrationObservation{}, new(fence.MaintenanceWindow), context.Background()) {
		t.Fatal("observation became original native Window")
	}
	if lifecycleEffectsPreflight(context.Background()) == nil {
		t.Fatal("inline leaf opened incomplete production effects")
	}
}
func TestPrepareStagingRefusesDeploymentFieldIncludingNull(t *testing.T) {
	for _, raw := range []string{`{"deployment_control":null}`, `{"deployment_control":{}}`} {
		if _, e := decodeLifecycleStagingRequest([]byte(raw)); e == nil {
			t.Fatal("prepare accepted deployment input")
		}
	}
}

func TestInlineAPIStaticBindingPreservesActualLeadingSlashName(t *testing.T) {
	v, r := inlineAPIExpectedFixture(t)
	w := stop.Container{ID: v.ID, Name: "/qs-apiserver", Image: v.Image, Entrypoint: []string{"/app/qs-apiserver"}, Command: []string{"--config", "/etc/qs-server/apiserver.yaml", "--migration.enabled=true"}, Project: "qs-server", Service: "apiserver"}
	if !lifecycleAPIStaticMatches(v, w, r.DeploymentControl.OriginalRuntimeSpecSHA256) {
		t.Fatal("actual slash name did not match the independent binding")
	}
	for _, name := range []string{"qs-apiserver", "/other-api"} {
		w.Name = name
		if lifecycleAPIStaticMatches(v, w, r.DeploymentControl.OriginalRuntimeSpecSHA256) {
			t.Fatal("normalized or different approved name accepted")
		}
	}
}
func TestInlineAPIInvocationRequiresEveryNonNullFieldAndLiteralFalse(t *testing.T) {
	v := lifecycleAPIInvocationIntent{}
	raw, e := json.Marshal(v)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = decodeLifecycleAPIInvocationIntent(raw); e != nil {
		t.Fatal("zero-valued input failed syntax-only decoding")
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		t.Fatal("bad input")
	}
	for name := range fields {
		t.Run(name+"_null", func(t *testing.T) {
			copyFields := map[string]json.RawMessage{}
			for k, v := range fields {
				copyFields[k] = v
			}
			copyFields[name] = json.RawMessage("null")
			changed, _ := json.Marshal(copyFields)
			if _, e := decodeLifecycleAPIInvocationIntent(changed); e == nil {
				t.Fatal("null provenance field accepted")
			}
		})
	}
	for _, value := range []string{"true", `"false"`, "0"} {
		fields["drop_authority"] = json.RawMessage(value)
		changed, _ := json.Marshal(fields)
		if _, e := decodeLifecycleAPIInvocationIntent(changed); e == nil {
			t.Fatal("non-literal false authority field accepted")
		}
	}
}
func TestInlineAPIStoppedReadbackChecksConfigAndActiveExecs(t *testing.T) {
	original, r := inlineAPIExpectedFixture(t)
	body, e := lifecycleAPICreateBody(original, r.DeploymentControl.BImageID, false, r)
	if e != nil {
		t.Fatal(e)
	}
	var expected map[string]json.RawMessage
	if json.Unmarshal(body, &expected) != nil {
		t.Fatal("bad payload")
	}
	actual := original
	actual.ID = strings.Repeat("6", 64)
	actual.Image = r.DeploymentControl.BImageID
	actual.Config = map[string]json.RawMessage{}
	for name := range original.Config {
		actual.Config[name] = expected[name]
	}
	actual.ExecIDs = json.RawMessage("null")
	if !lifecycleAPIStoppedRuntime(actual, original, actual.Image, digestRaw(body), body) {
		t.Fatal("pure expected stopped contract rejected")
	}
	actual.Config["Cmd"] = json.RawMessage(`["--migration.enabled=false"]`)
	if lifecycleAPIStoppedRuntime(actual, original, actual.Image, digestRaw(body), body) {
		t.Fatal("stopped container with changed command accepted")
	}
	actual.Config["Cmd"] = expected["Cmd"]
	for _, value := range []json.RawMessage{nil, json.RawMessage(`["active-exec"]`)} {
		actual.ExecIDs = value
		if lifecycleAPIStoppedRuntime(actual, original, actual.Image, digestRaw(body), body) {
			t.Fatal("missing or active daemon exec accepted")
		}
	}
	if lifecycleAPIJSONEqual([]byte(`9007199254740992`), []byte(`9007199254740993`)) {
		t.Fatal("large numeric config values collapsed")
	}
}

func TestInlineAPIImageSourceBindingKeepsBLabelAndLegacyRuntimeEvidence(t *testing.T) {
	source := strings.Repeat("a", 40)
	for _, c := range []struct {
		kind   string
		labels map[string]string
		want   bool
	}{
		{"b", nil, false}, {"b", map[string]string{"org.opencontainers.image.revision": source}, true},
		{"rollback", nil, true}, {"rollback", map[string]string{"org.opencontainers.image.revision": source}, true},
		{"rollback", map[string]string{"org.opencontainers.image.revision": ""}, false},
		{"rollback", map[string]string{"org.opencontainers.image.revision": strings.Repeat("b", 40)}, false},
	} {
		if lifecycleImageRevisionMatches(c.kind, c.labels, source) != c.want {
			t.Fatal("revision contract changed")
		}
	}
	settings := []debug.BuildSetting{{Key: "GOOS", Value: "linux"}, {Key: "GOARCH", Value: runtime.GOARCH}, {Key: "-ldflags", Value: "-s -w -X github.com/FangcunMount/qs-server/pkg/version.GitCommit=" + source}}
	if !lifecycleCompiledProgramSourceMatches(&debug.BuildInfo{Settings: settings}, source, runtime.GOARCH) {
		t.Fatal("actual compile settings rejected")
	}
	for _, change := range []func([]debug.BuildSetting) []debug.BuildSetting{
		func(v []debug.BuildSetting) []debug.BuildSetting { return v[:2] },
		func(v []debug.BuildSetting) []debug.BuildSetting { v[0].Value = "darwin"; return v },
		func(v []debug.BuildSetting) []debug.BuildSetting { v[1].Value = "unknown"; return v },
		func(v []debug.BuildSetting) []debug.BuildSetting {
			v[2].Value += " -X github.com/FangcunMount/qs-server/pkg/version.GitCommit=" + source
			return v
		},
		func(v []debug.BuildSetting) []debug.BuildSetting {
			v[2].Value = strings.ReplaceAll(v[2].Value, source, strings.Repeat("b", 40))
			return v
		},
		func(v []debug.BuildSetting) []debug.BuildSetting { return append(v, v[2]) },
	} {
		if lifecycleCompiledProgramSourceMatches(&debug.BuildInfo{Settings: change(append([]debug.BuildSetting(nil), settings...))}, source, runtime.GOARCH) {
			t.Fatal("ambiguous compiled source accepted")
		}
	}
	original, r := inlineAPIExpectedFixture(t)
	original.State.Running = true
	original.State.PID = 321
	original.State.StartedAt = "2026-10-10T00:00:00Z"
	build := []byte(`{"code":0,"message":"success","data":{"gitCommit":"` + source + `","platform":"linux/` + runtime.GOARCH + `"}}`)
	if !lifecycleOriginalRuntimeVersionMatches(original, original, original, r.DeploymentControl.RollbackImageID, source, build) {
		t.Fatal("same actual original instance rejected")
	}
	for _, change := range []func(*lifecycleAPIInspection){func(v *lifecycleAPIInspection) { v.State.PID++ }, func(v *lifecycleAPIInspection) { v.State.StartedAt = "later" }, func(v *lifecycleAPIInspection) { v.Image = r.DeploymentControl.BImageID }, func(v *lifecycleAPIInspection) { v.State.Running = false }} {
		after := original
		change(&after)
		if lifecycleOriginalRuntimeVersionMatches(original, after, original, r.DeploymentControl.RollbackImageID, source, build) {
			t.Fatal("runtime drift accepted")
		}
	}
	if lifecycleOriginalRuntimeVersionMatches(original, original, original, r.DeploymentControl.RollbackImageID, strings.Repeat("b", 40), build) {
		t.Fatal("wrong runtime source accepted")
	}
}
