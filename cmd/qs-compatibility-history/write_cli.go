package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	retirement "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirement"
)

// This report records returned database commits and an actual third readback.
// It is neither a portable qualification nor permission to retire storage.
type evidenceWriteReport struct {
	Protocol                     string `json:"protocol"`
	SourceSHA                    string `json:"source_sha"`
	ToolSourceSHA                string `json:"tool_source_sha"`
	OperationID                  string `json:"operation_id"`
	ActualRunID                  string `json:"actual_run_id"`
	RequestSHA256                string `json:"request_sha256"`
	DescriptorSHA256             string `json:"descriptor_sha256"`
	CommitState                  string `json:"commit_state"`
	MongoCommitRequirement       string `json:"mongo_commit_requirement"`
	AIOriginalCommands           uint64 `json:"ai_original_commands"`
	AISourceReferences           uint64 `json:"ai_source_references"`
	PreparedPages                uint64 `json:"prepared_pages"`
	ReadBackPages                uint64 `json:"readback_pages"`
	EventReferences              uint64 `json:"event_references"`
	ActualSQLCommitResponse      bool   `json:"actual_sql_commit_response"`
	ActualMongoCommitResponse    bool   `json:"actual_mongo_commit_response"`
	EventPersistenceObserved     bool   `json:"event_persistence_observed"`
	AICommandPersistenceComplete bool   `json:"ai_command_persistence_complete"`
	EvidenceWriteFinished        bool   `json:"evidence_write_finished"`
	WholeWriterFence             bool   `json:"whole_writer_fence"`
	FullExternalAIClosure        bool   `json:"full_external_ai_closure"`
	DropReady                    bool   `json:"drop_ready"`
	ErrorCategory                string `json:"error_category"`
	MaterialManifestSHA256       string `json:"material_manifest_sha256,omitempty"`
}

func parseEvidenceWriteFlags(args []string) (map[string]string, error) {
	want := map[string]bool{"write-mode": true, "request": true, "request-sha256": true,
		"operation": true, "run": true, "output": true, "operation-directory": true,
		"original-source-sha": true, "ai-input": true, "ai-input-sha256": true}
	out := map[string]string{}
	for i := 0; i < len(args); i += 2 {
		if i+1 >= len(args) || !strings.HasPrefix(args[i], "--") || strings.HasPrefix(args[i+1], "--") {
			return nil, fixedError("history_write_arguments_rejected")
		}
		key := strings.TrimPrefix(args[i], "--")
		if !want[key] || out[key] != "" || args[i+1] == "" {
			return nil, fixedError("history_write_arguments_rejected")
		}
		out[key] = args[i+1]
	}
	if len(out) != len(want) || out["write-mode"] != "evidence" || !sourcePattern.MatchString(sourceSHA) ||
		!sourcePattern.MatchString(out["original-source-sha"]) || !hashPattern.MatchString(out["request-sha256"]) ||
		!hashPattern.MatchString(out["ai-input-sha256"]) {
		return nil, fixedError("history_write_arguments_rejected")
	}
	if _, err := aiHostRun(out["run"]); err != nil {
		return nil, err
	}
	root := out["operation-directory"]
	registration := filepath.Join(root, "history-write-registration-"+out["run"])
	if aiHostOperationDirectory(root, out["operation"]) != nil ||
		out["request"] != filepath.Join(registration, "history.request.json") ||
		out["ai-input"] != filepath.Join(registration, "write.input.json") ||
		out["output"] != filepath.Join(root, "history-write-"+out["run"]) {
		return nil, fixedError("history_write_arguments_rejected")
	}
	return out, nil
}

func validateEvidenceWriteDescriptor(v aiHostDescriptor, f map[string]string) error {
	if v.Kind != "historical_evidence_write_host_input" || v.Mode != "write" ||
		v.SourceSHA != f["original-source-sha"] || v.ToolSourceSHA != sourceSHA {
		return fixedError("history_write_descriptor_rejected")
	}
	// Reuse the existing exact private/runtime/bounds constraint validator.
	// This local value conversion does not create any qualification or Q.
	v.Kind, v.Mode, v.SourceSHA, v.ToolSourceSHA = "readonly_ai_external_host_input", "verify", sourceSHA, ""
	verify := map[string]string{}
	for key, value := range f {
		verify[key] = value
	}
	verify["ai-host-mode"] = "verify"
	delete(verify, "original-source-sha")
	if validateAIHostDescriptor(v, verify) != nil {
		return fixedError("history_write_descriptor_rejected")
	}
	return nil
}

func evidenceWritePriorAttempt(directory string) error {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return fixedError("history_write_prior_attempt_unresolved")
	}
	for _, entry := range entries {
		// An unknown commit cannot be escaped by a different run or output name.
		// Reconciliation is a separate actual database procedure; no terminal JSON
		// here is sufficient to adopt a previous opaque plan or replay its write.
		if strings.HasPrefix(entry.Name(), "history-write-") &&
			!strings.HasPrefix(entry.Name(), "history-write-registration-") {
			return fixedError("history_write_prior_attempt_unresolved")
		}
	}
	if _, err := os.Lstat(filepath.Join(directory, "qs-ai-external-verify.exec.jsonl")); !os.IsNotExist(err) {
		// The write epoch must produce Q itself exactly once. A preceding readonly
		// verifier's saved summary cannot be imported or silently re-executed.
		return fixedError("history_write_prior_external_verifier_unresolved")
	}
	return nil
}

func applyWriteObservation(r *evidenceWriteReport, p preparedWriteDiagnostic) {
	r.CommitState, r.MongoCommitRequirement = p.CommitState, p.MongoCommitRequirement
	r.AIOriginalCommands, r.AISourceReferences = p.AIOriginalCommands, p.AISourceReferences
	r.PreparedPages, r.ReadBackPages, r.EventReferences = p.PreparedPages, p.ReadBackPages, p.EventReferences
	r.ActualSQLCommitResponse, r.ActualMongoCommitResponse = p.ActualSQLCommitResponse, p.ActualMongoCommitResponse
	r.EventPersistenceObserved, r.AICommandPersistenceComplete = p.LimitedEventPersistenceObserved, p.AICommandPersistenceComplete
	r.MaterialManifestSHA256 = p.MaterialManifestSHA256
}

func runEvidenceWriteCLI(ctx context.Context, args []string) (r evidenceWriteReport, result error) {
	r = evidenceWriteReport{Protocol: "qs-compatibility-evidence-write/v1", CommitState: "not_attempted", MongoCommitRequirement: "undetermined", ErrorCategory: "none"}
	f, err := parseEvidenceWriteFlags(args)
	if err != nil {
		return r, err
	}
	r.SourceSHA, r.ToolSourceSHA, r.OperationID, r.ActualRunID = f["original-source-sha"], sourceSHA, f["operation"], f["run"]
	r.RequestSHA256, r.DescriptorSHA256 = f["request-sha256"], f["ai-input-sha256"]
	lock, err := lockAIHostOperation(f["operation-directory"])
	if err != nil {
		return r, err
	}
	lockBefore, err := lock.Stat()
	if err != nil {
		_ = lock.Close()
		return r, fixedError("history_ai_host_operation_lock_rejected")
	}
	var assets []*aiHostPrivateAsset
	var a *approvedInputs
	var db *historyDatabase
	writeAttempted := false
	defer func() {
		if db != nil && db.close() != nil && result == nil {
			result = fixedError("history_connection_close_failed")
		}
		if a != nil && a.close() != nil && result == nil {
			result = fixedError("history_private_close_failed")
		}
		for _, asset := range assets {
			if asset.recheck() != nil && result == nil {
				result = fixedError("history_ai_host_private_input_changed")
			}
			if asset.file.Close() != nil && result == nil {
				result = fixedError("history_ai_host_private_close_failed")
			}
		}
		after, ae := lock.Stat()
		named, ne := os.Lstat(filepath.Join(f["operation-directory"], "operation.lock"))
		if (ae != nil || ne != nil || !aiHostSame(lockBefore, after) || !aiHostSame(lockBefore, named)) && result == nil {
			result = fixedError("history_ai_host_operation_lock_changed")
		}
		if lock.Close() != nil && result == nil {
			result = fixedError("history_ai_host_operation_close_failed")
		}
		r.EvidenceWriteFinished = evidenceWriteCompleted(r, result)
		if result == nil && !r.EvidenceWriteFinished {
			result = fixedError("history_write_completion_incomplete")
		}
		r.ErrorCategory = safeCategory(result)
		if writeAttempted {
			raw, me := json.Marshal(r)
			if me != nil || aiHostWriteFile(f["output"], "history.write.json", append(raw, '\n')) != nil {
				if result == nil {
					result = fixedError("history_write_result_record_failed")
				}
				r.EvidenceWriteFinished = false
				r.ErrorCategory = safeCategory(result)
			}
		}
	}()
	if err = evidenceWritePriorAttempt(f["operation-directory"]); err != nil {
		return r, err
	}
	input, err := readAIHostAsset(fileBinding{f["ai-input"], f["ai-input-sha256"]}, maximumJSONBytes)
	if err != nil {
		return r, err
	}
	assets = append(assets, input)
	var descriptor aiHostDescriptor
	if strictDecode(input.raw, &descriptor) != nil || validateEvidenceWriteDescriptor(descriptor, f) != nil {
		return r, fixedError("history_write_descriptor_rejected")
	}
	packets := map[string][]byte{}
	for _, item := range []struct {
		name string
		b    *fileBinding
		max  uint64
	}{{"ai", descriptor.AIBounds, 4 << 20}, {"peer", descriptor.PeerBounds, 4 << 20}, {"protection", descriptor.Protection, maximumJSONBytes}} {
		asset, err := readAIHostAsset(*item.b, item.max)
		if err != nil {
			return r, err
		}
		assets = append(assets, asset)
		packets[item.name] = asset.raw
	}
	a, err = loadInputsForSource(ctx, f["request"], f["request-sha256"], f["operation"], f["run"], f["original-source-sha"])
	if err != nil {
		return r, err
	}
	for _, path := range []string{a.request.InventoryRequest.Path, a.request.InventoryReport.Path} {
		if !aiHostUnder(f["operation-directory"], path) {
			return r, fixedError("history_ai_host_operation_directory_rejected")
		}
	}
	for _, asset := range a.request.Assets {
		if !aiHostUnder(f["operation-directory"], asset.Path) {
			return r, fixedError("history_ai_host_operation_directory_rejected")
		}
	}
	for _, asset := range assets {
		if asset.recheck() != nil {
			return r, fixedError("history_ai_host_private_input_changed")
		}
	}
	peer, err := aiHostPeerConnection()
	if err != nil {
		return r, err
	}
	db, err = openDatabases(ctx, a)
	if err != nil {
		return r, err
	}
	externalRun, _ := aiHostRun(f["run"])
	// The actual second read epoch constructs the sole Q, prepares native CAS
	// plans and applies them only after live admission/baseline locking checks.
	// No readonly summary or serialized capability enters this call.
	writeAttempted = true
	p, err := executeHistoricalEvidenceWrite(ctx, a, db, f["output"], retirement.AIExternalExecutionInput{
		OperationDirectory: f["operation-directory"], AssetsDirectory: descriptor.AssetsDirectory,
		RunID: externalRun, RuntimeSourceSHA: descriptor.RuntimeSourceSHA, ImageID: descriptor.ImageID,
		ContainerID: descriptor.ContainerID, ApprovedAIRuntimeBindingSHA256: descriptor.RuntimeBindingSHA256,
		AIBounds: packets["ai"], PeerBounds: packets["peer"], ApprovedAIBoundsSHA256: descriptor.AIBounds.SHA256,
		ApprovedPeerBoundsSHA256: descriptor.PeerBounds.SHA256, ProtectionJSON: packets["protection"],
		PeerConnection: peer, SudoDocker: true,
	})
	applyWriteObservation(&r, p)
	return r, err
}

func evidenceWriteCompleted(r evidenceWriteReport, result error) bool {
	if result != nil || !r.ActualSQLCommitResponse || r.ReadBackPages != r.PreparedPages ||
		(r.AIOriginalCommands != 0 && !r.AICommandPersistenceComplete) {
		return false
	}
	switch r.MongoCommitRequirement {
	case "required":
		return r.CommitState == "both_responses_success_non_atomic" && r.ActualMongoCommitResponse &&
			r.EventReferences > 0 && r.PreparedPages > 0 && r.EventPersistenceObserved
	case "not_required":
		return r.CommitState == "sql_committed_mongo_not_required" && !r.ActualMongoCommitResponse &&
			r.EventReferences == 0 && r.PreparedPages == 0 && !r.EventPersistenceObserved && r.AIOriginalCommands > 0
	default:
		return false
	}
}
