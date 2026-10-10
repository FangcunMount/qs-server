package retirement

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
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

func TestAIExternalRuntimeMountOrderPreservesExactSnapshotSemantics(t *testing.T) {
	in := AIExternalExecutionInput{RuntimeSourceSHA: strings.Repeat("a", 40), ImageID: "sha256:" + strings.Repeat("b", 64), ContainerID: strings.Repeat("c", 64)}
	before := aiExternalRuntime{ContainerID: in.ContainerID, ImageID: in.ImageID, Name: "/qs-ai", Status: "running", StartedAt: "2026-10-09T00:00:00Z", Running: true, ReadOnlyRoot: true, Command: []string{"/app/.venv/bin/python", "-m", "qs_ai.bootstrap.server"}, ContainerRevision: in.RuntimeSourceSHA, ImageRevision: in.RuntimeSourceSHA,
		Mounts: []aiExternalMount{{Type: "tmpfs", Destination: "/tmp", RW: true}, {Type: "bind", Source: "/private/qs-ai.key", Destination: "/run/qs-ai-tls/qs-ai.key"}, {Type: "bind", Source: "/private/ca-chain.crt", Destination: "/run/qs-ai-tls/ca-chain.crt"}}}
	if err := before.orderMounts(); err != nil || !before.matches(in) {
		t.Fatal("exact observed runtime rejected")
	}
	copyRuntime := func() aiExternalRuntime {
		v := before
		v.Mounts = append([]aiExternalMount(nil), before.Mounts...)
		v.Command = append([]string(nil), before.Command...)
		return v
	}
	after := copyRuntime()
	after.Mounts[0], after.Mounts[2] = after.Mounts[2], after.Mounts[0]
	if err := after.orderMounts(); err != nil || !after.matches(in) || !reflect.DeepEqual(before, after) {
		t.Fatal("unchanged mount set depended on inspect iteration order")
	}
	for _, row := range []struct {
		name   string
		mutate func(*aiExternalRuntime)
	}{
		{"source", func(v *aiExternalRuntime) { v.Mounts[0].Source = "/private/other.crt" }},
		{"type", func(v *aiExternalRuntime) { v.Mounts[0].Type = "volume" }},
		{"rw", func(v *aiExternalRuntime) { v.Mounts[0].RW = true }},
		{"destination", func(v *aiExternalRuntime) { v.Mounts[0].Destination = "/run/qs-ai-tls/qs-ai-fullchain.crt" }},
		{"container", func(v *aiExternalRuntime) { v.ContainerID = strings.Repeat("d", 64) }},
		{"image", func(v *aiExternalRuntime) { v.ImageID = "sha256:" + strings.Repeat("e", 64) }},
		{"started_at", func(v *aiExternalRuntime) { v.StartedAt = "2026-10-09T00:00:01Z" }},
		{"restart", func(v *aiExternalRuntime) { v.Restarts++ }},
		{"argv_order", func(v *aiExternalRuntime) { v.Command[0], v.Command[1] = v.Command[1], v.Command[0] }},
	} {
		t.Run(row.name, func(t *testing.T) {
			changed := copyRuntime()
			row.mutate(&changed)
			if err := changed.orderMounts(); err != nil {
				t.Fatal("distinct destinations unexpectedly rejected")
			}
			if reflect.DeepEqual(before, changed) {
				t.Fatal("normalization hid an observed runtime change")
			}
		})
	}
	t.Run("duplicate_destination", func(t *testing.T) {
		changed := copyRuntime()
		changed.Mounts = append(changed.Mounts, changed.Mounts[0])
		if err := changed.orderMounts(); err != ErrAIExternalRuntime || changed.matches(in) {
			t.Fatal("duplicate mount destination admitted")
		}
	})
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

func TestHistoricalAIQualificationRejectsImportedAndExpiredInputs(t *testing.T) {
	ctx := t.Context()
	if q, err := PrepareHistoricalAIExternalExecution(ctx, nil, nil, AIExternalExecutionInput{}); q != nil || err == nil {
		t.Fatal("missing actual inputs minted external qualification")
	}
	if b, err := PrepareHistoricalAICommandPersistenceBatch(ctx, nil, nil, &AIExternalExecutionQualification{}); b != nil || err == nil {
		t.Fatal("external DTO minted persistence batch")
	}
	if err := ValidateHistoricalComponentAI(ctx, nil, &AIExternalExecutionQualification{}, &AICommandPersistenceBatch{}); err == nil {
		t.Fatal("empty component/receipt bypassed actual AI closure")
	}
	var b AICommandPersistenceBatch
	if _, err := b.VerifyHistoricalReadback(ctx, nil, nil); err == nil {
		t.Fatal("empty batch claimed committed server read")
	}
	q := &AIExternalExecutionQualification{}
	q.self = q
	if q.historicalIntact(ctx, nil, nil) == nil {
		t.Fatal("self pointer alone minted qualification")
	}
	if _, err := prepareHistoricalAIFullSnapshot(ctx, nil, nil, DefaultAIReverseLimits(), "READ ONLY"); err == nil {
		t.Fatal("full14 observed without original pair")
	}
}
func TestHistoricalAIWholeLedgerEqualityPreservesActualPhysicalFacts(t *testing.T) {
	ledgers := make([]AIReverseLedgerSummary, len(aiReverseSpecs))
	for i, spec := range aiReverseSpecs {
		ledgers[i] = AIReverseLedgerSummary{Store: spec.table, Rows: 1, Bytes: 8, Pages: 1, SchemaSHA256: "schema", PrimaryKeySHA256: "pk", UpperSHA256: "upper", RowsSHA256: "actual"}
	}
	got := append([]AIReverseLedgerSummary(nil), ledgers...)
	got[0].Pages = 9
	if !historicalAILedgersEqual(got, ledgers) {
		t.Fatal("pagination was mistaken for changed content")
	}
	for _, mutate := range []func(*AIReverseLedgerSummary){func(v *AIReverseLedgerSummary) { v.Rows++ }, func(v *AIReverseLedgerSummary) { v.Bytes++ }, func(v *AIReverseLedgerSummary) { v.RowsSHA256 = "changed" }, func(v *AIReverseLedgerSummary) { v.SchemaSHA256 = "changed" }, func(v *AIReverseLedgerSummary) { v.PrimaryKeySHA256 = "changed" }, func(v *AIReverseLedgerSummary) { v.UpperSHA256 = "changed" }, func(v *AIReverseLedgerSummary) { v.Store = "outside" }} {
		changed := append([]AIReverseLedgerSummary(nil), ledgers...)
		mutate(&changed[3])
		if historicalAILedgersEqual(changed, ledgers) {
			t.Fatal("real row/catalog/upper change was hidden")
		}
	}
	if historicalAILedgersEqual(got[:13], ledgers) {
		t.Fatal("missing whole ledger accepted")
	}
}

func TestHistoricalAIKnownPendingDoesNotAdoptReceiptsOrHeldWork(t *testing.T) {
	q := &AIExternalExecutionQualification{handoffs: map[string]aiExternalKnownHandoff{"original": {CommandID: "original", RequestID: "request"}}}
	for _, store := range []string{"ai_messaging_inbox", "ai_messaging_failures", AIBridgeCommandSource} {
		n := &aiReverseNode{id: "original", command: "original", state: "staged", observation: AIReverseObservation{Store: store, Unfinished: true}}
		if historicalAIKnownCurrentPending(n, q) {
			t.Fatal("mapped ID adopted another delivery responsibility")
		}
	}
	n := &aiReverseNode{id: "original", command: "original", state: "staged", observation: AIReverseObservation{Store: "ai_messaging_outbox", Unfinished: true}}
	if !historicalAIKnownCurrentPending(n, q) {
		t.Fatal("known current staged handoff lost")
	}
	n.observation.Held = true
	if historicalAIKnownCurrentPending(n, q) {
		t.Fatal("held handoff was adopted")
	}
	n.observation.Held = false
	n.state = "confirmed"
	if historicalAIKnownCurrentPending(n, q) {
		t.Fatal("unexpected transport state was adopted")
	}
}

func TestHistoricalAIScopedContinuityKeepsNegativeRangesAndIgnoresUnrelatedRows(t *testing.T) {
	current := &AIReverseSnapshot{metadata: make([]aiReverseMetadata, len(aiReverseSpecs)), scope: &aiReverseScope{relatedRequests: map[string]bool{"request": true}, relatedIDs: map[string]bool{"command": true}}, componentAssessments: map[string]bool{"42": true}, componentResources: map[string]bool{"resource": true}, byTable: map[string]map[string]*aiReverseNode{}}
	committed := &AIReverseSnapshot{metadata: make([]aiReverseMetadata, len(aiReverseSpecs)), byTable: map[string]map[string]*aiReverseNode{}}
	for _, spec := range aiReverseSpecs {
		current.byTable[spec.table], committed.byTable[spec.table] = map[string]*aiReverseNode{}, map[string]*aiReverseNode{}
	}
	row := func(table, id, request, resource, hash string) *aiReverseNode {
		return &aiReverseNode{id: id, request: request, resource: resource, observation: AIReverseObservation{Store: table, PrimaryKeySHA256: "pk:" + id, RowSHA256: hash}}
	}
	related := row("ai_messaging_operations", "command", "", "resource", "committed-retirement")
	current.byTable[related.observation.Store][related.id] = related
	committed.byTable[related.observation.Store][related.id] = related
	committed.byTable["ai_messaging_operations"]["unrelated"] = row("ai_messaging_operations", "unrelated", "", "other-resource", "other-current-business")
	if !historicalAIScopedRowsEqual(current, committed) {
		t.Fatal("unrelated legitimate business row blocked selected continuity")
	}
	delete(current.byTable[related.observation.Store], related.id)
	if historicalAIScopedRowsEqual(current, committed) {
		t.Fatal("disappeared related operation hidden")
	}
	current.byTable[related.observation.Store][related.id] = row(related.observation.Store, related.id, "", "resource", "changed-org-or-payload")
	if historicalAIScopedRowsEqual(current, committed) {
		t.Fatal("changed selected raw row accepted")
	}
	current.byTable[related.observation.Store][related.id] = related
	current.byTable["ai_messaging_operations"]["new-command"] = row("ai_messaging_operations", "new-command", "", "resource", "new-current-responsibility")
	if historicalAIScopedRowsEqual(current, committed) {
		t.Fatal("new matching responsibility omitted from expected image")
	}
	delete(current.byTable["ai_messaging_operations"], "new-command")
	absent := row("ai_bridge_request_assessments", "request:42", "request", "42", "original-negative-range")
	committed.byTable[absent.observation.Store][absent.id] = absent
	if historicalAIScopedRowsEqual(current, committed) {
		t.Fatal("request/assessment negative expansion was cut")
	}
	delete(committed.byTable[absent.observation.Store], absent.id)
	admission := row("ai_messaging_admission", "1", "", "", "closed-revision")
	committed.byTable[admission.observation.Store][admission.id] = admission
	if historicalAIScopedRowsEqual(current, committed) {
		t.Fatal("missing actual admission control accepted")
	}
	current.byTable[admission.observation.Store][admission.id] = admission
	if !historicalAIScopedRowsEqual(current, committed) {
		t.Fatal("same closed selected image rejected")
	}
}
