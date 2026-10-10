package retirement

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestAIExternalPublicZeroAndImportedDTOCannotCreateQualification(t *testing.T) {
	var q AIExternalExecutionQualification
	if err := json.Unmarshal([]byte(`{"complete":true,"self":true,"drop_ready":true,"originals":8}`), &q); !errors.Is(err, ErrSourceSerialization) {
		t.Fatal("opaque qualification accepted imported JSON")
	}
	if q.Summary().ExternalDatabaseFactsObserved || q.Summary().DropReady || q.Summary().CASAuthority {
		t.Fatal("imported summary created authority")
	}
	if _, err := json.Marshal(&q); !errors.Is(err, ErrSourceSerialization) {
		t.Fatal("opaque qualification serialized")
	}
	c := &HistoricalCoordinator{}
	if result, err := c.PrepareAIExternalExecution(context.Background(), nil, nil, AIExternalExecutionInput{}); result != nil || err != ErrAILocalBinding {
		t.Fatal("missing actual snapshots admitted")
	}
	if result, err := c.PrepareAIExternalPage(context.Background(), nil, &q); result != nil || err == nil {
		t.Fatal("empty/imported qualification admitted a page")
	}
}

func TestAIExternalPrivateInputNeverFormatsCredentials(t *testing.T) {
	peer := AIExternalPeerConnection{Host: "fixture.invalid", Port: 3306, Database: "synthetic", Username: "do-not-publish-user", Password: "do-not-publish-password"}
	in := AIExternalExecutionInput{PeerConnection: peer, ProtectionJSON: []byte(`{"key":"do-not-publish-key"}`)}
	for _, v := range []any{in, peer} {
		if strings.Contains(fmt.Sprintf("%v %#v", v, v), "do-not-publish") {
			t.Fatal("private input formatted")
		}
		if _, err := json.Marshal(v); !errors.Is(err, ErrSourceSerialization) {
			t.Fatal("private input serialized")
		}
	}
}

func TestAIExternalProducerSyntaxRejectsFlagsDuplicatesAndEOFJunk(t *testing.T) {
	for _, raw := range []string{
		`{"protocol":"qs-ai-actual-execution-facts/v2","drop_ready":true}`,
		`{"protocol":"qs-ai-actual-execution-facts/v2","source_sha":"a","source_sha":"b"}`,
		`{"protocol":"qs-ai-actual-execution-facts/v2"} {"complete":true}`,
		`{"protocol":"qs-ai-actual-execution-failed/v1","category":"execution_rejected"}`,
	} {
		if _, err := aiExternalDecodeActualResult([]byte(raw)); err == nil {
			t.Fatal("untrusted syntax admitted")
		}
	}
}

func TestAIExternalWholePeerRowsCannotHideChangedDuplicateOrMissingNodes(t *testing.T) {
	pk := strings.Repeat("a", 64)
	row := strings.Repeat("b", 64)
	s := &AIReverseSnapshot{nodes: []*aiReverseNode{{observation: AIReverseObservation{Store: "ai_bridge_commands", PrimaryKeySHA256: pk, RowSHA256: row}}}}
	if aiExternalPeerRowsMatch([]aiExternalPeerRow{{Store: "ai_bridge_commands", PrimaryKeySHA: pk, RowSHA: row}}, s) != nil {
		t.Fatal("exact raw row rejected")
	}
	for _, rows := range [][]aiExternalPeerRow{nil,
		{{Store: "ai_bridge_commands", PrimaryKeySHA: pk, RowSHA: strings.Repeat("c", 64)}},
		{{Store: "ai_bridge_commands", PrimaryKeySHA: pk, RowSHA: row}, {Store: "ai_bridge_commands", PrimaryKeySHA: pk, RowSHA: row}},
		{{Store: "different", PrimaryKeySHA: pk, RowSHA: row}},
	} {
		if aiExternalPeerRowsMatch(rows, s) == nil {
			t.Fatal("whole reverse raw baseline changed")
		}
	}
}

func TestAIExternalRuntimeRequiresActualImmutableSourceAndStableContainer(t *testing.T) {
	in := AIExternalExecutionInput{RuntimeSourceSHA: strings.Repeat("a", 40), ImageID: "sha256:" + strings.Repeat("b", 64), ContainerID: strings.Repeat("c", 64)}
	r := aiExternalRuntime{ContainerID: in.ContainerID, ImageID: in.ImageID, Name: "/qs-ai", Status: "running", StartedAt: "2026-10-09T00:00:00Z", Running: true, ReadOnlyRoot: true, Command: []string{"/app/.venv/bin/python", "-m", "qs_ai.bootstrap.server"}, ContainerRevision: in.RuntimeSourceSHA, ImageRevision: in.RuntimeSourceSHA}
	if !r.matches(in) {
		t.Fatal("exact binding rejected")
	}
	for _, mutate := range []func(*aiExternalRuntime){func(v *aiExternalRuntime) { v.Running = false }, func(v *aiExternalRuntime) { v.ImageRevision = strings.Repeat("d", 40) }, func(v *aiExternalRuntime) { v.ContainerID = strings.Repeat("e", 64) }, func(v *aiExternalRuntime) { v.Name = "/different" }, func(v *aiExternalRuntime) { v.StartedAt = "unknown" }, func(v *aiExternalRuntime) { v.ReadOnlyRoot = false }, func(v *aiExternalRuntime) { v.Entrypoint = []string{"arbitrary"} }, func(v *aiExternalRuntime) {
		v.Mounts = []aiExternalMount{{Type: "bind", Destination: "/app", RW: false}}
	}, func(v *aiExternalRuntime) {
		v.Mounts = []aiExternalMount{{Type: "bind", Destination: "/usr/local/bin/python", RW: false}}
	}} {
		changed := r
		mutate(&changed)
		if changed.matches(in) {
			t.Fatal("unobserved/wrong runtime admitted")
		}
	}
	r.Mounts = []aiExternalMount{{Type: "bind", Destination: "/run/qs-ai-tls/qs-ai.key"}, {Type: "tmpfs", Destination: "/tmp", RW: true}}
	if !r.matches(in) {
		t.Fatal("actual compose mount contract rejected")
	}
	r.Mounts[0].RW = true
	if r.matches(in) {
		t.Fatal("writable protected runtime mount admitted")
	}
}

func TestAIExternalCurrentObservationKeepsSettingsPrivateAndCannotStop(t *testing.T) {
	in := AIExternalExecutionInput{RuntimeSourceSHA: strings.Repeat("a", 40), ImageID: "sha256:" + strings.Repeat("b", 64), ContainerID: strings.Repeat("c", 64)}
	snap := aiStoppedSnapshot{Runtime: aiExternalRuntime{ContainerID: in.ContainerID, ImageID: in.ImageID, Name: "/qs-ai", Status: "running", StartedAt: "2026-10-09T00:00:00Z", Running: true, ReadOnlyRoot: true, ContainerRevision: in.RuntimeSourceSHA, ImageRevision: in.RuntimeSourceSHA, Command: []string{"/app/.venv/bin/python", "-m", "qs_ai.bootstrap.server"}}, PID: 123, RestartPolicy: "unless-stopped", NetworkID: strings.Repeat("d", 64), Settings: map[string]string{"QS_AI_DATABASE_URL": "synthetic-private-setting"}, HealthcheckTest: []string{"CMD", "/app/.venv/bin/python", "-c", "import urllib.request; urllib.request.urlopen('http://127.0.0.1:8000/readyz', timeout=3)"}}
	binding := strings.Repeat("e", 64)
	observed, err := aiExternalObservationFromSnapshot(in, binding, snap)
	if err != nil || observed.SourceSHA != in.RuntimeSourceSHA || observed.ImageID != in.ImageID || observed.ContainerID != in.ContainerID || observed.BindingSHA256 != binding || observed.StopConstraints.SettingsSHA256 != sourceSHA(aiJSONBytes(snap.Settings)) || !observed.StopConstraints.Valid() {
		t.Fatal("actual snapshot projection rejected")
	}
	raw, err := json.Marshal(observed)
	if err != nil || strings.Contains(string(raw), "synthetic-private-setting") || strings.Contains(string(raw), "drop_ready") || strings.Contains(string(raw), "lease") {
		t.Fatal("settings or authority escaped observation")
	}
	for _, mutate := range []func(*aiStoppedSnapshot){func(v *aiStoppedSnapshot) { v.Runtime.Running = false }, func(v *aiStoppedSnapshot) { v.Runtime.ContainerRevision = strings.Repeat("f", 40) }, func(v *aiStoppedSnapshot) { v.PID = 0 }, func(v *aiStoppedSnapshot) { v.RestartPolicy = "always" }, func(v *aiStoppedSnapshot) { v.NetworkID = "unobserved" }, func(v *aiStoppedSnapshot) { v.ExecIDs = []string{"unsettled-original-exec"} }, func(v *aiStoppedSnapshot) { v.HealthcheckTest = nil }, func(v *aiStoppedSnapshot) { v.Paused = true }, func(v *aiStoppedSnapshot) { v.Settings = nil }} {
		changed := snap
		mutate(&changed)
		if _, err = aiExternalObservationFromSnapshot(in, binding, changed); err == nil {
			t.Fatal("unproven runtime/constraint accepted")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, work := range []context.Context{nil, ctx} {
		if _, err = ObserveAIExternalCurrentRuntime(work, false); err != ErrAIExternalInput {
			t.Fatal("missing live context reached native reader")
		}
	}
}

func TestAIExternalReleaseOwnerUsesNativeRootActorAndActualNonRootUID(t *testing.T) {
	for _, row := range []struct {
		euid   int
		native string
		want   uint32
	}{{0, "1001", 1001}, {0, "0", 0}, {0, "", 0}, {1001, "0", 1001}, {1001, "not-an-authenticated-uid", 1001}} {
		got, err := aiExternalReleaseOwnerUIDValue(row.euid, row.native)
		if err != nil || got != row.want {
			t.Fatal("native root actor or actual non-root identity changed")
		}
	}
	for _, value := range []string{"-1", "+1001", "01001", "1001\n", "4294967296", "user", " ", "null"} {
		if _, err := aiExternalReleaseOwnerUIDValue(0, value); err == nil {
			t.Fatal("noncanonical native actor accepted")
		}
	}
}

func TestAIExternalPrivateResultOutputHasHardBound(t *testing.T) {
	writer := &aiExternalBoundedOutput{limit: 3}
	if n, e := writer.Write([]byte("123")); n != 3 || e != nil {
		t.Fatal("limit boundary rejected")
	}
	if n, e := writer.Write([]byte("4")); n != 0 || e == nil || writer.Len() != 3 {
		t.Fatal("private output limit exceeded")
	}
}

func TestAIExternalActualMQRewritePreservesBaseTLSAndBindsEveryJOSEFile(t *testing.T) {
	// The actual deployment generator emits these four individual read-only
	// bindings. Compose merges them with the base TLS targets, not /app or a
	// directory mount. This is a structural test, not real deployment evidence.
	raw := `{"enabled":true,"nsqd":{"nsqd-a:4150":"http://nsqd-a:4151"},"signing_key_file":"/run/qs-ai-jose/ai.sign.v1.json","decrypt_key_files":{"ai.encrypt.v1":"/run/qs-ai-jose/ai.encrypt.v1.json"},"qs_signer_files":{"qs.sign.v1":"/run/qs-ai-jose/qs.sign.v1.json"},"qs_recipient_key_file":"/run/qs-ai-jose/qs.encrypt.v1.json","max_in_flight":1}`
	volumes := []aiExternalReleaseVolume{}
	actual := []aiExternalMount{{Type: "bind", Source: "/data/infra/ssl/grpc/ca/ca-chain.crt", Destination: "/run/qs-ai-tls/ca-chain.crt"}, {Type: "bind", Source: "/data/infra/ssl/grpc/server/qs-ai-fullchain.crt", Destination: "/run/qs-ai-tls/qs-ai-fullchain.crt"}, {Type: "bind", Source: "/data/infra/ssl/grpc/server/qs-ai.key", Destination: "/run/qs-ai-tls/qs-ai.key"}}
	for _, name := range []string{"ai.sign.v1.json", "ai.encrypt.v1.json", "qs.sign.v1.json", "qs.encrypt.v1.json"} {
		v := aiExternalReleaseVolume{ReadOnly: true, Source: "/data/infra/qs-ai-messaging/versions/review-v1/" + name, Target: "/run/qs-ai-jose/" + name, Type: "bind"}
		volumes = append(volumes, v)
		actual = append(actual, aiExternalMount{Type: v.Type, Source: v.Source, Destination: v.Target})
	}
	config, err := aiExternalReleaseMounts(raw, volumes)
	if err != nil || string(config) != raw {
		t.Fatal("actual MQ rewrite rejected")
	}
	r := aiExternalRelease{messaging: config, mounts: volumes}
	if !r.matchesMounts(actual) {
		t.Fatal("compose base and MQ unique targets rejected")
	}
	for _, mutate := range []func([]aiExternalMount) []aiExternalMount{
		func(v []aiExternalMount) []aiExternalMount { v[3].RW = true; return v },
		func(v []aiExternalMount) []aiExternalMount {
			v[3].Source = "/data/infra/qs-ai-messaging/versions/other/ai.sign.v1.json"
			return v
		},
		func(v []aiExternalMount) []aiExternalMount {
			return append(v, aiExternalMount{Type: "bind", Source: "/tmp/source", Destination: "/app"})
		},
		func(v []aiExternalMount) []aiExternalMount { return v[:len(v)-1] },
		func(v []aiExternalMount) []aiExternalMount { return append(v, v[3]) },
	} {
		if r.matchesMounts(mutate(append([]aiExternalMount(nil), actual...))) {
			t.Fatal("changed or partial actual mounts admitted")
		}
	}
	changed := append([]aiExternalReleaseVolume(nil), volumes...)
	changed[0].Source = "/data/infra/qs-ai-messaging/versions/other/ai.sign.v1.json"
	if _, err = aiExternalReleaseMounts(raw, changed); err == nil {
		t.Fatal("mixed release key versions admitted")
	}
	changed = append([]aiExternalReleaseVolume(nil), volumes...)
	changed[0].Target = "/app/key.json"
	if _, err = aiExternalReleaseMounts(raw, changed); err == nil {
		t.Fatal("app shadow authorized")
	}
}
