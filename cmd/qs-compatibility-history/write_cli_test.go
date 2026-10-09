package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func evidenceCLIInputs(t *testing.T) ([]string, map[string]string) {
	t.Helper()
	previous := sourceSHA
	sourceSHA = strings.Repeat("b", 40)
	t.Cleanup(func() { sourceSHA = previous })
	root := filepath.Join(aiHostUnitPrivateDir(t), "backups", "qs-server", "compatibility-retirement", "100-1")
	if os.MkdirAll(root, 0700) != nil {
		t.Fatal("directory")
	}
	registration := filepath.Join(root, "history-write-registration-101-1")
	f := map[string]string{"write-mode": "evidence", "request": filepath.Join(registration, "history.request.json"),
		"request-sha256": strings.Repeat("c", 64), "operation": "100-1", "run": "101-1",
		"output": filepath.Join(root, "history-write-101-1"), "operation-directory": root,
		"original-source-sha": strings.Repeat("a", 40), "ai-input": filepath.Join(registration, "write.input.json"),
		"ai-input-sha256": strings.Repeat("d", 64)}
	var args []string
	for _, key := range []string{"write-mode", "request", "request-sha256", "operation", "run", "output",
		"operation-directory", "original-source-sha", "ai-input", "ai-input-sha256"} {
		args = append(args, "--"+key, f[key])
	}
	return args, f
}

func evidenceDescriptor(t *testing.T, f map[string]string) aiHostDescriptor {
	t.Helper()
	assets := aiHostUnitPrivateDir(t)
	if os.Chmod(assets, 0700) != nil {
		t.Fatal("mode")
	}
	root := f["operation-directory"]
	return aiHostDescriptor{FormatVersion: 1, Kind: "historical_evidence_write_host_input", Mode: "write",
		SourceSHA: f["original-source-sha"], ToolSourceSHA: sourceSHA, OperationID: f["operation"], ActualRunID: f["run"],
		RequestSHA256: f["request-sha256"], RuntimeSourceSHA: strings.Repeat("e", 40), ImageID: "sha256:" + strings.Repeat("f", 64),
		ContainerID: strings.Repeat("1", 64), RuntimeBindingSHA256: strings.Repeat("2", 64), AssetsDirectory: assets,
		AIBounds:   &fileBinding{filepath.Join(root, "ai.bounds.json"), strings.Repeat("3", 64)},
		PeerBounds: &fileBinding{filepath.Join(root, "peer.bounds.json"), strings.Repeat("4", 64)},
		Protection: &fileBinding{filepath.Join(root, "ai-message-protection.json"), strings.Repeat("5", 64)}}
}

func TestEvidenceWriteCLIRequiresExactNewInputs(t *testing.T) {
	args, _ := evidenceCLIInputs(t)
	if _, err := parseEvidenceWriteFlags(args); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range [][]string{args[:len(args)-2], append(append([]string{}, args...), "--request", "/tmp/other"),
		append(append([]string{}, args...), "--whole-writer-fence", "true")} {
		if _, err := parseEvidenceWriteFlags(invalid); err == nil {
			t.Fatal("missing, duplicate or claimed permission admitted")
		}
	}
	wrong := append([]string{}, args...)
	wrong[1] = "prepare"
	if _, err := parseEvidenceWriteFlags(wrong); err == nil {
		t.Fatal("read-only mode confused with write")
	}
	wrong = append([]string{}, args...)
	wrong[11] += "-another"
	if _, err := parseEvidenceWriteFlags(wrong); err == nil {
		t.Fatal("unbound output allowed")
	}
}

func TestEvidenceWriteDescriptorSeparatesOriginalAndToolWithoutProof(t *testing.T) {
	_, f := evidenceCLIInputs(t)
	v := evidenceDescriptor(t, f)
	if validateEvidenceWriteDescriptor(v, f) != nil {
		t.Fatal("explicit original/tool constraints rejected")
	}
	v.ToolSourceSHA = v.SourceSHA
	if validateEvidenceWriteDescriptor(v, f) == nil {
		t.Fatal("original A relabelled as execution tool")
	}
	v = evidenceDescriptor(t, f)
	raw, _ := json.Marshal(v)
	for _, extra := range []string{`,"writer_fence":true}`, `,"qualification":{"complete":true}}`, `,"tool_source_sha":"` + sourceSHA + `"}`} {
		var decoded aiHostDescriptor
		if strictDecode(append(raw[:len(raw)-1], []byte(extra)...), &decoded) == nil {
			t.Fatal("serialized proof or duplicate accepted")
		}
	}
}

func TestEvidenceSourceBindingDoesNotRelaxDefaultReadOnlyCaller(t *testing.T) {
	_, f := evidenceCLIInputs(t)
	registration := filepath.Dir(f["request"])
	if os.Mkdir(registration, 0700) != nil {
		t.Fatal("directory")
	}
	v := historyRequest{FormatVersion: 1, Kind: "readonly_compatibility_history_request", SourceSHA: f["original-source-sha"],
		OperationID: f["operation"], RunID: f["run"], Assets: make([]assetBinding, 4)}
	raw, _ := json.Marshal(v)
	if os.WriteFile(f["request"], raw, 0600) != nil {
		t.Fatal("input")
	}
	digest := rawHash(raw)
	if _, err := loadInputs(context.Background(), f["request"], digest, f["operation"], f["run"]); safeCategory(err) != "history_request_binding_rejected" {
		t.Fatal("default readonly source binding changed", err)
	}
	if _, err := loadInputsForSource(context.Background(), f["request"], digest, f["operation"], f["run"], f["original-source-sha"]); safeCategory(err) != "history_inventory_input_rejected" {
		t.Fatal("explicit original source did not reach actual inventory validation", err)
	}
}

func TestEvidenceWritePriorAttemptCannotEscapeWithNewRun(t *testing.T) {
	_, f := evidenceCLIInputs(t)
	root := f["operation-directory"]
	if evidenceWritePriorAttempt(root) != nil {
		t.Fatal("empty operation rejected")
	}
	old := filepath.Join(root, "history-write-99-1")
	if os.Mkdir(old, 0700) != nil {
		t.Fatal("directory")
	}
	if evidenceWritePriorAttempt(root) == nil {
		t.Fatal("different run escaped old write uncertainty")
	}
	if os.Remove(old) != nil || os.WriteFile(filepath.Join(root, "qs-ai-external-verify.exec.jsonl"), []byte("{}\n"), 0600) != nil {
		t.Fatal("files")
	}
	if evidenceWritePriorAttempt(root) == nil {
		t.Fatal("prior readonly verifier mistaken for reusable Q")
	}
}

func TestEvidenceCommitUnknownRemainsVisibleWithoutCompletion(t *testing.T) {
	r := evidenceWriteReport{}
	applyWriteObservation(&r, preparedWriteDiagnostic{CommitState: "sql_committed_mongo_unknown", ActualSQLCommitResponse: true,
		AIOriginalCommands: 12, PreparedPages: 7})
	if !r.ActualSQLCommitResponse || r.ActualMongoCommitResponse || r.CommitState != "sql_committed_mongo_unknown" ||
		r.AIOriginalCommands != 12 || r.EvidenceWriteFinished || r.DropReady || r.WholeWriterFence || r.FullExternalAIClosure {
		t.Fatal("commit uncertainty hidden or promoted")
	}
}

func TestOriginalAIBoundsSourceExtensionCannotAuthorizeVerifier(t *testing.T) {
	_, f := evidenceCLIInputs(t)
	v := evidenceDescriptor(t, f)
	v.Kind, v.Mode = "readonly_ai_external_host_input", "bounds"
	v.AIBounds, v.PeerBounds, v.Protection = nil, nil, nil
	f["ai-host-mode"] = "bounds"
	if validateAIHostDescriptor(v, f) != nil {
		t.Fatal("explicit original-source bounds rejected")
	}
	f["ai-host-mode"] = "verify"
	if validateAIHostDescriptor(v, f) == nil {
		t.Fatal("readonly verifier allowed to relabel original source")
	}
	f["ai-host-mode"] = "bounds"
	delete(f, "original-source-sha")
	if validateAIHostDescriptor(v, f) == nil {
		t.Fatal("default readonly path imported new source override")
	}
	_, args := aiHostUnitFlags(t, "bounds")
	args = append(args, "--original-source-sha", strings.Repeat("a", 40))
	if _, err := parseAIHostFlags(args); err != nil {
		t.Fatal("explicit bounds flag rejected", err)
	}
	args[1] = "verify"
	if _, err := parseAIHostFlags(args); err == nil {
		t.Fatal("source override accepted by readonly verifier flags")
	}
}
