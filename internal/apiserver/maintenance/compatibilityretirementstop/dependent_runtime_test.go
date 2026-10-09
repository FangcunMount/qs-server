package compatibilityretirementstop

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	redisobs "github.com/FangcunMount/qs-server/internal/pkg/redisruntime/observability"
)

func TestControlledProtocolIsRemoteOnlyAndKeepsLegacyActions(t *testing.T) {
	for _, action := range []string{"controlled_resume", "check_running"} {
		raw, _ := json.Marshal(SessionRequest{sessionProtocol, 2, action})
		if _, e := parseRemoteSessionRequest(raw, 2); e != nil {
			t.Fatal(e)
		}
		if _, e := parseSessionRequest(raw, 2); e == nil {
			t.Fatal("new action entered legacy local producer")
		}
		for _, refused := range []bool{false, true} {
			if remoteSessionActionAllowed(action, true, true, refused, false) == refused {
				t.Fatal("new action escaped original stop/refusal state")
			}
		}
	}
	for _, l := range []*Lease{nil, {}} {
		if l.ControlledResumeDependents(context.Background()) == nil {
			t.Fatal("unissued lease resumed services")
		}
		if o, e := l.ObserveRunningDependents(context.Background()); e == nil || o != nil {
			t.Fatal("unissued lease produced runtime evidence")
		}
	}
	if _, e := json.Marshal(new(DependentRuntimeObservation)); e == nil {
		t.Fatal("opaque runtime observation serialized")
	}
}

func TestRunningReadbackRejectsNewIDsAndUnknownResume(t *testing.T) {
	base, actual, _, _ := stoppedDependentsFixture()
	actual[0].Running, actual[0].PID, actual[0].StartedAt = true, 77, "2026-10-10T01:02:03Z"
	restored := map[string]string{actual[0].ID: actual[0].StartedAt}
	if _, e := runningDependents(base, actual, restored, "server-a"); e != nil {
		t.Fatal(e)
	}
	for name, change := range map[string]func([]actualContainer){
		"new-cid":     func(v []actualContainer) { v[0].ID = strings.Repeat("9", 64) },
		"new-image":   func(v []actualContainer) { v[0].Image = "sha256:" + strings.Repeat("9", 64) },
		"wrong-start": func(v []actualContainer) { v[0].StartedAt = "unissued" },
		"pid-zero":    func(v []actualContainer) { v[0].PID = 0 },
		"stopped":     func(v []actualContainer) { v[0].Running = false },
		"restarting":  func(v []actualContainer) { v[0].Restarting = true },
	} {
		t.Run(name, func(t *testing.T) {
			v := append([]actualContainer(nil), actual...)
			change(v)
			if _, e := runningDependents(base, v, restored, "server-a"); e == nil {
				t.Fatal("runtime drift accepted")
			}
		})
	}
	if _, e := runningDependents(base, actual, nil, "server-a"); e == nil {
		t.Fatal("unknown native resume was accepted")
	}
	extra := append(append([]actualContainer(nil), actual...), actualContainer{Container: base[0]})
	if _, e := runningDependents(base, extra, restored, "server-a"); e == nil {
		t.Fatal("residual API hidden")
	}
	workers := append([]Container(nil), base[1:]...)
	workers[0].Component, workers[0].Project, workers[0].Service = "qs-worker", "qs-worker", "runtime"
	workers[0].Entrypoint = []string{"/app/qs-worker"}
	workers[0].Command = []string{"--config=/app/configs/worker.prod.yaml"}
	w := actual[0]
	w.Container = workers[0]
	w.Running, w.StartedAt = true, restored[w.ID]
	if _, e := runningDependents(workers, []actualContainer{w}, restored, "server-d"); e != nil {
		t.Fatal("same native D worker rejected")
	}
	if _, e := runningDependents(workers, append([]actualContainer{w}, actual[1]), restored, "server-d"); e == nil {
		t.Fatal("API on worker host hidden")
	}
}

func TestRuntimeExecutableAndVersionFixedContracts(t *testing.T) {
	program := "/app/qs-worker"
	h := strings.Repeat("a", 64)
	if got, e := runtimeProgramHash(h+"  /proc/1/exe\n"+h+"  "+program+"\n", program); e != nil || got != h {
		t.Fatal("native process hash contract rejected")
	}
	for _, raw := range []string{h + "  /proc/1/exe\n" + strings.Repeat("b", 64) + "  " + program + "\n", h + "  " + program + "\n", h + "  /proc/1/exe\n" + h + "  " + program + "\nextra"} {
		if _, e := runtimeProgramHash(raw, program); e == nil {
			t.Fatal("mismatched executable hashes accepted")
		}
	}
	source := strings.Repeat("b", 40)
	version := "ignored startup output\ngitVersion: v1\ngitCommit: " + source + "\ngitTreeState: clean\nbuildDate: 2026-10-10\ngoVersion: go1.25.12\ncompiler: gc\nplatform: linux/amd64\n"
	if e := runtimeVersionValid(version, source, "amd64"); e != nil {
		t.Fatal(e)
	}
	for name, raw := range map[string]string{"wrong-source": strings.Replace(version, source, strings.Repeat("c", 40), 1), "dirty": strings.Replace(version, "clean", "dirty", 1), "wrong-arch": strings.Replace(version, "amd64", "arm64", 1), "duplicate": version + "gitCommit: " + source + "\n", "extra-block": version + version} {
		t.Run(name, func(t *testing.T) {
			if runtimeVersionValid(raw, source, "amd64") == nil {
				t.Fatal("unbound build accepted")
			}
		})
	}
	for _, mount := range []string{"/app", "/app/qs-worker", "/proc", "/proc/1"} {
		if runtimeMountsSafe([]string{mount}, program) {
			t.Fatal("process/binary shadowing accepted")
		}
	}
	if !runtimeMountsSafe([]string{"/app/configs", "/etc/qs-messaging"}, program) {
		t.Fatal("original config mounts rejected")
	}
	for _, raw := range []string{"C /app/qs-worker\n", "A /app\n", "D /\n", "malformed"} {
		if runtimeDiffSafe(raw, program) == nil {
			t.Fatal("modified original binary accepted")
		}
	}
	if runtimeDiffSafe("C /tmp/log\n", program) != nil || runtimeDiffSafe("", program) != nil {
		t.Fatal("unrelated native diff rejected")
	}
}

func TestRuntimeActualReadinessNeverMeansMQOrStaticFallback(t *testing.T) {
	start := time.Now().Add(-time.Minute).UTC().Format(time.RFC3339Nano)
	snapshot := redisobs.RuntimeSnapshot{GeneratedAt: time.Now(), Component: "worker", Families: []redisobs.FamilyStatus{{Component: "worker", Family: "current", Configured: true, Available: true}}}
	snapshot.Summary = redisobs.SummarizeFamilies(snapshot.Families)
	valid := func(s redisobs.RuntimeSnapshot) []byte {
		b, e := json.Marshal(struct {
			Status    string                   `json:"status"`
			Component string                   `json:"component"`
			Redis     redisobs.RuntimeSnapshot `json:"redis"`
		}{"ready", "worker", s})
		if e != nil {
			t.Fatal(e)
		}
		return b
	}
	if runtimeReadinessValid(valid(snapshot), "worker", start) != nil {
		t.Fatal("real readiness wire contract rejected")
	}
	for name, change := range map[string]func(*redisobs.RuntimeSnapshot){"empty": func(s *redisobs.RuntimeSnapshot) { s.Families = nil; s.Summary = redisobs.RuntimeSummary{Ready: true} }, "stale": func(s *redisobs.RuntimeSnapshot) { s.GeneratedAt = time.Now().Add(-time.Hour) }, "wrong-summary": func(s *redisobs.RuntimeSnapshot) { s.Summary.AvailableCount = 0 }, "degraded": func(s *redisobs.RuntimeSnapshot) {
		s.Families = append([]redisobs.FamilyStatus(nil), s.Families...)
		s.Families[0].Degraded = true
	}} {
		t.Run(name, func(t *testing.T) {
			v := snapshot
			change(&v)
			if runtimeReadinessValid(valid(v), "worker", start) == nil {
				t.Fatal("static/degraded/stale readiness became runtime proof")
			}
		})
	}
	if runtimeReadinessValid([]byte(`{"status":"ready","component":"worker","mq_verified":true}`), "worker", start) == nil {
		t.Fatal("imported MQ success accepted")
	}
	if !runtimeReplyFits(descriptorFixture().Containers) {
		t.Fatal("current topology exceeds unchanged wire bound")
	}
	base := descriptorFixture().Containers
	for i := 0; i < 32; i++ {
		base = append(base, base[1])
	}
	if runtimeReplyFits(base) {
		t.Fatal("session metadata bound expanded")
	}
}

func TestCollectionActualServeReadinessHasSeparateDependencyGate(t *testing.T) {
	start := time.Now().Add(-time.Minute).UTC().Format(time.RFC3339Nano)
	snapshot := redisobs.RuntimeSnapshot{GeneratedAt: time.Now(), Component: "collection-server", Families: []redisobs.FamilyStatus{{Component: "collection-server", Family: "current", Configured: true, Available: true}}}
	snapshot.Summary = redisobs.SummarizeFamilies(snapshot.Families)
	data := map[string]any{"status": "ready", "service": "collection-server", "version": "v1", "serve_ready": true, "dependency_ready": true, "redis": snapshot, "resilience_control_synchronized": true}
	encode := func(data map[string]any) []byte {
		b, e := json.Marshal(map[string]any{"code": 0, "message": "success", "data": data})
		if e != nil {
			t.Fatal(e)
		}
		return b
	}
	if runtimeReadinessValid(encode(data), "collection-server", start) != nil {
		t.Fatal("actual normal collection handler response rejected")
	}
	for _, key := range []string{"serve_ready", "dependency_ready", "resilience_control_synchronized"} {
		t.Run(key, func(t *testing.T) {
			v := map[string]any{}
			for k, x := range data {
				v[k] = x
			}
			v[key] = false
			if runtimeReadinessValid(encode(v), "collection-server", start) == nil {
				t.Fatal("serving gate hid unavailable dependency")
			}
		})
	}
	if runtimeReadinessValid(encode(data), "worker", start) == nil {
		t.Fatal("wrong component response accepted")
	}
}
