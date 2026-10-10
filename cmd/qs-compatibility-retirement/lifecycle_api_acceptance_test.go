package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	backup "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementbackup"
	"github.com/FangcunMount/qs-server/pkg/version"
)

func TestAPIAcceptanceFixedReadRouteRequiresActualUnambiguousContainerNetwork(t *testing.T) {
	v := lifecycleAPIInspection{Config: map[string]json.RawMessage{"ExposedPorts": json.RawMessage(`{"8080/tcp":{},"8443/tcp":{}}`)}}
	v.NetworkSettings.Networks = map[string]json.RawMessage{"approved": json.RawMessage(`{"IPAddress":"172.20.0.2","NetworkID":"` + strings.Repeat("a", 64) + `"}`)}
	if address, e := lifecycleAPIReadAddress(v); e != nil || address != "172.20.0.2:8080" {
		t.Fatalf("actual fixed native route lost: %q %v", address, e)
	}
	for _, raw := range []string{`{"IPAddress":"127.0.0.1","NetworkID":"` + strings.Repeat("a", 64) + `"}`, `{"IPAddress":"8.8.8.8","NetworkID":"` + strings.Repeat("a", 64) + `"}`, `{"IPAddress":"172.20.0.2","NetworkID":"unbound"}`} {
		v.NetworkSettings.Networks["approved"] = json.RawMessage(raw)
		if _, e := lifecycleAPIReadAddress(v); e == nil {
			t.Fatal("unproven network route accepted")
		}
	}
	v.NetworkSettings.Networks["approved"] = json.RawMessage(`{"IPAddress":"172.20.0.2","NetworkID":"` + strings.Repeat("a", 64) + `"}`)
	v.NetworkSettings.Networks["second"] = v.NetworkSettings.Networks["approved"]
	if _, e := lifecycleAPIReadAddress(v); e == nil {
		t.Fatal("ambiguous networks were guessed")
	}
}

func TestAPIAcceptanceCurrentSourceAndActualDependencyReadinessCannotBeStaticHealth(t *testing.T) {
	source := strings.Repeat("a", 40)
	health := []byte(`{"code":0,"message":"success","data":{"status":"ok"}}`)
	build := []byte(`{"code":0,"message":"success","data":{"gitCommit":"` + source + `","gitTreeState":"` + version.GitTreeState + `","platform":"linux/amd64"}}`)
	ready := []byte(`{"status":"ready","component":"apiserver","redis":{"generated_at":"` + time.Now().UTC().Format(time.RFC3339Nano) + `","component":"apiserver","summary":{"ready":true,"family_total":1,"available_count":1},"families":[{"component":"apiserver","family":"current_runtime","configured":true,"available":true}]}}`)
	if e := lifecycleAPIValidateResponses(health, build, ready, source, "amd64"); e != nil {
		t.Fatal(e)
	}
	for name, raw := range map[string][]byte{
		"fallback-empty-families": []byte(`{"status":"ready","component":"apiserver","redis":{"generated_at":"` + time.Now().UTC().Format(time.RFC3339Nano) + `","component":"apiserver","summary":{"ready":true},"families":[]}}`),
		"static-health":           []byte(`{"status":"healthy","version":"1.0.0"}`),
		"degraded":                []byte(strings.Replace(string(ready), `"ready":true`, `"ready":false`, 1)),
	} {
		t.Run(name, func(t *testing.T) {
			if e := lifecycleAPIValidateResponses(health, build, raw, source, "amd64"); e == nil {
				t.Fatal("static/fallback/degraded response became dependency proof")
			}
		})
	}
	for _, badBuild := range [][]byte{[]byte(strings.Replace(string(build), source, strings.Repeat("b", 40), 1)), []byte(strings.Replace(string(build), `"gitTreeState":"`+version.GitTreeState+`"`, `"gitTreeState":"dirty"`, 1)), []byte(strings.Replace(string(build), "linux/amd64", "linux/arm64", 1)), []byte(`{"code":0,"code":0,"message":"success","data":{}}`)} {
		if e := lifecycleAPIValidateResponses(health, badBuild, ready, source, "amd64"); e == nil {
			t.Fatal("wrong or ambiguous deployed source accepted")
		}
	}
}

type acceptanceTestTransport func(*http.Request) (*http.Response, error)

func (f acceptanceTestTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestAPIAcceptanceReadEndpointsAreBoundedGETOnly(t *testing.T) {
	for _, status := range []int{200, 302, 503} {
		client := &http.Client{Transport: acceptanceTestTransport(func(r *http.Request) (*http.Response, error) {
			if r.Method != http.MethodGet || r.URL.Host != "172.20.0.2:8080" || r.URL.Path != "/version" {
				t.Fatal("unexpected endpoint or write")
			}
			return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"source":"observed"}`))}, nil
		})}
		if _, e := lifecycleAPIReadEndpoint(t.Context(), client, "172.20.0.2:8080", "/version"); (e == nil) != (status == 200) {
			t.Fatal("redirect/unavailable response accepted")
		}
	}
	client := &http.Client{Transport: acceptanceTestTransport(func(*http.Request) (*http.Response, error) {
		t.Fatal("unsupported endpoint reached transport")
		return nil, nil
	})}
	if _, e := lifecycleAPIReadEndpoint(t.Context(), client, "172.20.0.2:8080", "/admin/write"); e == nil {
		t.Fatal("arbitrary route accepted")
	}
}

func TestAPIAcceptanceRepeatedReadKeepsOriginalNativeInstance(t *testing.T) {
	v := &lifecycleAPITransition{bCID: strings.Repeat("a", 64)}
	actual := lifecycleAPIInspection{ID: v.bCID, Image: "sha256:" + strings.Repeat("b", 64)}
	actual.State.PID, actual.State.StartedAt = 17, "2026-10-10T00:00:00Z"
	source, address := strings.Repeat("c", 40), "172.20.0.2:8080"
	o := &lifecycleAPIRuntimeObservation{owner: v, cid: actual.ID, image: actual.Image, source: source, pid: actual.State.PID, started: actual.State.StartedAt, address: address}
	o.self = o
	if !o.matchesCurrent(v, actual, address, source) {
		t.Fatal("same native API instance could not be read again")
	}
	for name, change := range map[string]func(*lifecycleAPIRuntimeObservation){
		"foreign-owner": func(x *lifecycleAPIRuntimeObservation) { x.owner = new(lifecycleAPITransition) },
		"saved-copy":    func(x *lifecycleAPIRuntimeObservation) { x.self = o },
		"new-cid":       func(x *lifecycleAPIRuntimeObservation) { x.cid = strings.Repeat("d", 64) },
		"new-image":     func(x *lifecycleAPIRuntimeObservation) { x.image = "sha256:" + strings.Repeat("d", 64) },
		"wrong-source":  func(x *lifecycleAPIRuntimeObservation) { x.source = strings.Repeat("d", 40) },
		"new-pid":       func(x *lifecycleAPIRuntimeObservation) { x.pid++ },
		"restarted":     func(x *lifecycleAPIRuntimeObservation) { x.started = "2026-10-10T00:01:00Z" },
		"new-route":     func(x *lifecycleAPIRuntimeObservation) { x.address = "172.20.0.3:8080" },
	} {
		t.Run(name, func(t *testing.T) {
			x := *o
			x.self = &x
			change(&x)
			if x.matchesCurrent(v, actual, address, source) {
				t.Fatal("changed API runtime accepted as original instance")
			}
		})
	}
	// Each cycle goes through the existing fixed GET caller. A later failure
	// remains an error rather than being satisfied by the first response.
	reads := 0
	client := &http.Client{Transport: acceptanceTestTransport(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodGet || req.URL.Path != "/readyz" {
			t.Fatal("repeat read was not the fixed GET")
		}
		reads++
		status := 200
		if reads == 2 {
			status = 503
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(`{"status":"ready"}`))}, nil
	})}
	if _, err := lifecycleAPIReadEndpoint(t.Context(), client, address, "/readyz"); err != nil {
		t.Fatal(err)
	}
	if _, err := lifecycleAPIReadEndpoint(t.Context(), client, address, "/readyz"); err == nil || reads != 2 {
		t.Fatal("repeat GET was skipped or old success masked current failure")
	}
	if err := v.observeAcceptance(context.Background(), lifecycleRequest{ToolSourceSHA: source}); err == nil || v.acceptance != nil {
		t.Fatal("missing native inputs produced acceptance")
	}
}

func TestAPIMaterialRegistrationFailureRetainsActualWrittenBytesAndUnknown(t *testing.T) {
	// Real current-UID filesystem refusal. Nonroot bytes cannot become a
	// root registration. In a true root test process, exercise O_EXCL refusal
	// on an existing owned file instead; neither case fabricates API authority.
	materials := materialTestDirectory(t, filepath.Join(t.TempDir(), "actual-owner"))
	dir := materials.path
	if os.Getuid() == 0 {
		if e := os.WriteFile(filepath.Join(dir, "owned-spec.private.json"), []byte("exact-owned-original-spec"), 0600); e != nil {
			t.Fatal(e)
		}
	}
	v := &lifecycleAPITransition{dir: dir, materials: materials}
	v.self = v
	if e := v.writeMaterial("owned-spec.private.json", []byte("exact-owned-original-spec")); e == nil || !v.unknown || !materials.unknown {
		t.Fatal("unregistered write produced a reusable owner")
	}
	raw, e := os.ReadFile(filepath.Join(dir, "owned-spec.private.json"))
	if e != nil || string(raw) != "exact-owned-original-spec" {
		t.Fatal("uncertain material was deleted/replaced")
	}
	if e := v.writeMaterial("owned-spec.private.json", []byte("replacement")); e == nil {
		t.Fatal("unknown original write replayed")
	}
}

func TestNativeAcceptanceCannotMintMaterialsFromImportedPlanOrHealthyDTO(t *testing.T) {
	h := &lifecycleFixedHost{owner: &lifecyclePreparationOwner{}, dataBaseline: new(backup.NonTargetDataBaseline), acceptancePlan: new(backup.TargetRecoveryPlan), services: new(lifecycleServiceController), api: new(lifecycleAPITransition)}
	if h.VerifyAcceptance(context.Background(), lifecycleRequest{}, new(backup.Archive)) == nil || h.acceptedMaterials != nil {
		t.Fatal("unissued native data/runtime owner minted purge permission")
	}
	if h.BindAcceptancePlan(context.Background(), lifecycleRequest{}, new(backup.Archive), new(backup.TargetRecoveryPlan)) == nil {
		t.Fatal("budget/imported plan became live native acceptance binding")
	}
}

func TestAPIProgramProbeInheritsExactImageLabelsWithoutOwnerConflict(t *testing.T) {
	owned := map[string]string{"codex.task": "qs-compatibility-retirement", "codex.operation": "op", "codex.run": "7", "codex.tool_source": "source", "codex.kind": "api-program-b"}
	image := map[string]string{"org.opencontainers.image.revision": "actual-source", "other.original": "kept"}
	labels, err := lifecycleAPIProgramProbeLabels(image, owned)
	if err != nil || len(labels) != 7 || labels["org.opencontainers.image.revision"] != "actual-source" || labels["other.original"] != "kept" {
		t.Fatal("actual image labels were lost", err)
	}
	for key, value := range owned {
		if labels[key] != value {
			t.Fatal("owner key lost")
		}
	}
	if len(image) != 2 {
		t.Fatal("original image labels mutated")
	}
	image["codex.run"] = "other-run"
	if _, err = lifecycleAPIProgramProbeLabels(image, owned); err == nil {
		t.Fatal("conflicting inherited owner accepted")
	}
	image["codex.run"] = "7"
	if _, err = lifecycleAPIProgramProbeLabels(image, owned); err != nil {
		t.Fatal("matching original owner rejected", err)
	}
}
