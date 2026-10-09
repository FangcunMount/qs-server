package main

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	sqlevaluation "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	retirement "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirement"
)

// These are independently hash-bound constraints on an actual host observation.
// No report, complete bit, terminal assertion, SQL, broker endpoint or imported
// qualification is accepted. Peer credentials remain in the existing host env.
type aiHostDescriptor struct {
	FormatVersion          int          `json:"format_version"`
	Kind                   string       `json:"kind"`
	SourceSHA              string       `json:"source_sha"`
	ToolSourceSHA          string       `json:"tool_source_sha,omitempty"`
	OperationID            string       `json:"operation_id"`
	ActualRunID            string       `json:"actual_run_id"`
	Mode                   string       `json:"mode"`
	RequestSHA256          string       `json:"request_sha256"`
	RuntimeSourceSHA       string       `json:"runtime_source_sha"`
	ImageID                string       `json:"image_id"`
	ContainerID            string       `json:"container_id"`
	RuntimeBindingSHA256   string       `json:"approved_runtime_binding_sha256"`
	AssetsDirectory        string       `json:"assets_directory"`
	ExpectedAIIdentityHash string       `json:"expected_ai_identity_hash"`
	ExpectedAIHead         string       `json:"expected_ai_head"`
	AIBounds               *fileBinding `json:"ai_bounds,omitempty"`
	PeerBounds             *fileBinding `json:"peer_bounds,omitempty"`
	Protection             *fileBinding `json:"protection,omitempty"`
}

type aiHostReport struct {
	Protocol                     string                              `json:"protocol"`
	Mode                         string                              `json:"mode"`
	SourceSHA                    string                              `json:"source_sha"`
	ToolSourceSHA                string                              `json:"tool_source_sha,omitempty"`
	OperationID                  string                              `json:"operation_id"`
	ActualRunID                  string                              `json:"actual_run_id"`
	ExternalRunID                string                              `json:"external_run_id"`
	RequestSHA256                string                              `json:"request_sha256"`
	DescriptorSHA256             string                              `json:"descriptor_sha256"`
	ExpectedRuntimeBindingSHA256 string                              `json:"expected_runtime_binding_sha256"`
	RuntimeBindingSHA256         string                              `json:"runtime_binding_sha256"`
	Bounds                       *retirement.AIExternalBoundsSummary `json:"bounds,omitempty"`
	Verification                 *aiHostVerificationSummary          `json:"verification,omitempty"`
	ErrorCategory                string                              `json:"error_category"`
	DiagnosticOnly               bool                                `json:"diagnostic_only"`
	DiagnosticReadComplete       bool                                `json:"diagnostic_read_complete"`
	Complete                     bool                                `json:"complete"`
	IndependentApproval          bool                                `json:"independent_approval"`
	WholeWriterFence             bool                                `json:"whole_writer_fence"`
	CASAuthority                 bool                                `json:"cas_authority"`
	RetirementWritten            bool                                `json:"retirement_written"`
	DropReady                    bool                                `json:"drop_ready"`
	RequiredAdapters             []string                            `json:"required_adapters"`
}

type aiHostVerificationSummary struct {
	Scope                         string `json:"scope"`
	Originals                     uint64 `json:"originals"`
	FactsSHA256                   string `json:"facts_sha256"`
	IndependentEpochs             int    `json:"independent_epochs"`
	ExternalDatabaseFactsObserved bool   `json:"external_database_facts_observed"`
	ProductionApproval            bool   `json:"production_approval"`
	WriterFence                   bool   `json:"writer_fence"`
	BrokerCoverage                bool   `json:"broker_coverage"`
	CASAuthority                  bool   `json:"cas_authority"`
	DropReady                     bool   `json:"drop_ready"`
}

type aiHostPrivateAsset struct {
	path   string
	file   *os.File
	before os.FileInfo
	hash   string
	max    uint64
	raw    []byte
}

// New host descriptors/bounds use nonblocking fd validation. A replaced FIFO
// cannot hold the entire SSH job outside its context deadline.
func readAIHostAsset(binding fileBinding, max uint64) (*aiHostPrivateAsset, error) {
	if !hashPattern.MatchString(binding.SHA256) || privateParent(binding.Path) != nil {
		return nil, fixedError("history_ai_host_private_input_rejected")
	}
	fd, e := syscall.Open(binding.Path, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if e != nil {
		return nil, fixedError("history_ai_host_private_input_rejected")
	}
	f := os.NewFile(uintptr(fd), binding.Path)
	failed := func() (*aiHostPrivateAsset, error) {
		_ = f.Close()
		return nil, fixedError("history_ai_host_private_input_rejected")
	}
	st, e := f.Stat()
	if e != nil || !aiHostRegular(st, max) {
		return failed()
	}
	raw, e := io.ReadAll(io.LimitReader(f, int64(max)+1))
	named, ne := os.Lstat(binding.Path)
	after, ae := f.Stat()
	if e != nil || ne != nil || ae != nil || uint64(len(raw)) > max || rawHash(raw) != binding.SHA256 || !aiHostSame(st, after) || !aiHostSame(st, named) {
		return failed()
	}
	return &aiHostPrivateAsset{path: binding.Path, file: f, before: st, hash: binding.SHA256, max: max, raw: raw}, nil
}
func aiHostRegular(st os.FileInfo, max uint64) bool {
	if st == nil || !st.Mode().IsRegular() || st.Mode().Perm() != 0600 || st.Size() < 0 || uint64(st.Size()) > max {
		return false
	}
	s, ok := st.Sys().(*syscall.Stat_t)
	return ok && s.Uid == uint32(os.Geteuid()) && s.Nlink == 1
}
func aiHostSame(before, after os.FileInfo) bool {
	if before == nil || after == nil || !os.SameFile(before, after) || before.Mode() != after.Mode() || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		return false
	}
	b, bok := before.Sys().(*syscall.Stat_t)
	a, aok := after.Sys().(*syscall.Stat_t)
	return bok && aok && b.Uid == a.Uid && b.Gid == a.Gid && b.Nlink == a.Nlink && a.Nlink == 1
}
func (a *aiHostPrivateAsset) recheck() error {
	if a == nil || a.file == nil {
		return fixedError("history_ai_host_private_input_changed")
	}
	before, e := a.file.Stat()
	named, ne := os.Lstat(a.path)
	if e != nil || ne != nil || !aiHostRegular(before, a.max) || !aiHostSame(a.before, before) || !aiHostSame(a.before, named) {
		return fixedError("history_ai_host_private_input_changed")
	}
	if _, e = a.file.Seek(0, io.SeekStart); e != nil {
		return fixedError("history_ai_host_private_input_changed")
	}
	raw, e := io.ReadAll(io.LimitReader(a.file, int64(a.max)+1))
	after, ae := a.file.Stat()
	if e != nil || ae != nil || rawHash(raw) != a.hash || !aiHostSame(a.before, after) {
		return fixedError("history_ai_host_private_input_changed")
	}
	return nil
}
func aiHostUnder(root, path string) bool {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return false
	}
	rel, e := filepath.Rel(root, path)
	return e == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
func aiHostOperationDirectory(directory, op string) error {
	if !runPattern.MatchString(op) || !filepath.IsAbs(directory) || filepath.Clean(directory) != directory || filepath.Base(directory) != op || filepath.Base(filepath.Dir(directory)) != "compatibility-retirement" || filepath.Base(filepath.Dir(filepath.Dir(directory))) != "qs-server" || filepath.Base(filepath.Dir(filepath.Dir(filepath.Dir(directory)))) != "backups" || privateParent(filepath.Join(directory, "host-input.json")) != nil {
		return fixedError("history_ai_host_operation_directory_rejected")
	}
	return nil
}
func aiHostRun(actual string) (string, error) {
	if !runPattern.MatchString(actual) {
		return "", fixedError("history_ai_host_run_binding_rejected")
	}
	pieces := strings.Split(actual, "-")
	n, e := strconv.ParseUint(pieces[0], 10, 64)
	attempt, ae := strconv.ParseUint(pieces[1], 10, 16)
	if e != nil || ae != nil || n == 0 || attempt == 0 || strconv.FormatUint(n, 10) != pieces[0] || strconv.FormatUint(attempt, 10) != pieces[1] {
		return "", fixedError("history_ai_host_run_binding_rejected")
	}
	return pieces[0], nil
}
func parseAIHostFlags(args []string) (map[string]string, error) {
	want := map[string]bool{"ai-host-mode": true, "request": true, "request-sha256": true, "operation": true, "run": true, "output": true, "operation-directory": true, "ai-input": true, "ai-input-sha256": true}
	out := map[string]string{}
	for i := 0; i < len(args); i += 2 {
		if i+1 >= len(args) || !strings.HasPrefix(args[i], "--") || strings.HasPrefix(args[i+1], "--") {
			return nil, fixedError("history_ai_host_arguments_rejected")
		}
		k := strings.TrimPrefix(args[i], "--")
		if (!want[k] && k != "original-source-sha") || out[k] != "" || args[i+1] == "" {
			return nil, fixedError("history_ai_host_arguments_rejected")
		}
		out[k] = args[i+1]
	}
	extended := out["original-source-sha"] != ""
	if extended && (out["ai-host-mode"] != "bounds" || !sourcePattern.MatchString(out["original-source-sha"])) {
		return nil, fixedError("history_ai_host_arguments_rejected")
	}
	expectedCount := len(want)
	if extended {
		expectedCount++
	}
	if len(out) != expectedCount || (out["ai-host-mode"] != "bounds" && out["ai-host-mode"] != "verify") || !hashPattern.MatchString(out["request-sha256"]) || !hashPattern.MatchString(out["ai-input-sha256"]) {
		return nil, fixedError("history_ai_host_arguments_rejected")
	}
	if _, e := aiHostRun(out["run"]); e != nil {
		return nil, e
	}
	if aiHostOperationDirectory(out["operation-directory"], out["operation"]) != nil || !aiHostUnder(out["operation-directory"], out["request"]) || !aiHostUnder(out["operation-directory"], out["ai-input"]) || out["output"] != filepath.Join(out["operation-directory"], "ai-host-"+out["ai-host-mode"]+"-"+out["run"]) {
		return nil, fixedError("history_ai_host_arguments_rejected")
	}
	return out, nil
}
func validateAIHostDescriptor(v aiHostDescriptor, f map[string]string) error {
	expectedSource := sourceSHA
	if f["original-source-sha"] != "" {
		if f["ai-host-mode"] != "bounds" || !sourcePattern.MatchString(f["original-source-sha"]) || v.ToolSourceSHA != sourceSHA {
			return fixedError("history_ai_host_descriptor_rejected")
		}
		expectedSource = f["original-source-sha"]
	} else if v.ToolSourceSHA != "" {
		return fixedError("history_ai_host_descriptor_rejected")
	}
	if v.FormatVersion != 1 || v.Kind != "readonly_ai_external_host_input" || v.SourceSHA != expectedSource || v.OperationID != f["operation"] || v.ActualRunID != f["run"] || v.Mode != f["ai-host-mode"] || v.RequestSHA256 != f["request-sha256"] || !sourcePattern.MatchString(v.RuntimeSourceSHA) || !strings.HasPrefix(v.ImageID, "sha256:") || !hashPattern.MatchString(strings.TrimPrefix(v.ImageID, "sha256:")) || !hashPattern.MatchString(v.ContainerID) || !hashPattern.MatchString(v.RuntimeBindingSHA256) || !filepath.IsAbs(v.AssetsDirectory) || filepath.Clean(v.AssetsDirectory) != v.AssetsDirectory || privateParent(filepath.Join(v.AssetsDirectory, "asset")) != nil {
		return fixedError("history_ai_host_descriptor_rejected")
	}
	if (v.ExpectedAIIdentityHash == "") != (v.ExpectedAIHead == "") || v.ExpectedAIIdentityHash != "" && (!hashPattern.MatchString(v.ExpectedAIIdentityHash) || v.ExpectedAIHead != "0038_messaging_observations" && v.ExpectedAIHead != "0040_module_table_names") {
		return fixedError("history_ai_host_descriptor_rejected")
	}
	if v.Mode == "bounds" {
		if v.AIBounds != nil || v.PeerBounds != nil || v.Protection != nil {
			return fixedError("history_ai_host_descriptor_rejected")
		}
	} else {
		if v.AIBounds == nil || v.PeerBounds == nil || v.Protection == nil || v.ExpectedAIIdentityHash != "" || v.ExpectedAIHead != "" {
			return fixedError("history_ai_host_descriptor_rejected")
		}
		for _, b := range []*fileBinding{v.AIBounds, v.PeerBounds, v.Protection} {
			if !hashPattern.MatchString(b.SHA256) || !aiHostUnder(f["operation-directory"], b.Path) {
				return fixedError("history_ai_host_descriptor_rejected")
			}
		}
	}
	return nil
}

// Same operation.lock as the existing Python host. This native entry owns it;
// the SSH wrapper must not retain an outer flock while waiting on this process.
// It serializes this operation only and is not a writer/platform fence.
func lockAIHostOperation(directory string) (*os.File, error) {
	path := filepath.Join(directory, "operation.lock")
	fd, e := syscall.Open(path, syscall.O_RDWR|syscall.O_CREAT|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0600)
	if e != nil {
		return nil, fixedError("history_ai_host_operation_lock_rejected")
	}
	f := os.NewFile(uintptr(fd), path)
	before, e := f.Stat()
	named, ne := os.Lstat(path)
	if e != nil || ne != nil || !aiHostRegular(before, 0) || !aiHostSame(before, named) || syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		_ = f.Close()
		return nil, fixedError("history_ai_host_operation_lock_rejected")
	}
	return f, nil
}
func aiHostWriteFile(directory, name string, raw []byte) error {
	path := filepath.Join(directory, name)
	if filepath.Base(name) != name || privateParent(path) != nil {
		return fixedError("history_ai_host_private_output_rejected")
	}
	fd, e := syscall.Open(path, syscall.O_WRONLY|syscall.O_CREAT|syscall.O_EXCL|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0600)
	if e != nil {
		return fixedError("history_ai_host_private_output_rejected")
	}
	f := os.NewFile(uintptr(fd), path)
	n, e := f.Write(raw)
	se := f.Sync()
	ce := f.Close()
	dir, de := os.Open(directory)
	if de != nil {
		return fixedError("history_ai_host_private_output_rejected")
	}
	ds := dir.Sync()
	dc := dir.Close()
	if e != nil || n != len(raw) || se != nil || ce != nil || ds != nil || dc != nil {
		return fixedError("history_ai_host_private_output_rejected")
	}
	return nil
}
func aiHostPeerConnection() (retirement.AIExternalPeerConnection, error) {
	var v retirement.AIExternalPeerConnection
	var e error
	if v.Host, e = connectionValue("MYSQL_HOST"); e != nil {
		return v, e
	}
	if v.Database, e = connectionValue("MYSQL_DATABASE"); e != nil {
		return v, e
	}
	if v.Username, e = connectionValue("MYSQL_USERNAME"); e != nil {
		return v, e
	}
	if v.Password, e = connectionValue("MYSQL_PASSWORD"); e != nil {
		return v, e
	}
	v.Port, e = connectionPort("MYSQL_PORT", 3306)
	return v, e
}
func aiHostCategory(e error) error {
	if e == nil {
		return nil
	}
	switch e {
	case retirement.ErrAIExternalInput:
		return fixedError("history_ai_host_external_input_rejected")
	case retirement.ErrAIExternalRuntime:
		return fixedError("history_ai_host_runtime_binding_rejected")
	case retirement.ErrAIExternalBounds:
		return fixedError("history_ai_host_bounds_failed_or_execution_unknown")
	case retirement.ErrAIExternalExecution:
		return fixedError("history_ai_host_verifier_failed_or_execution_unknown")
	case retirement.ErrAIExternalChanged:
		return fixedError("history_ai_host_external_facts_changed")
	case retirement.ErrAIExternalExecUnknown:
		return fixedError("history_ai_host_prior_execution_unknown")
	case retirement.ErrAIExternalExecJournal:
		return fixedError("history_ai_host_prior_journal_rejected")
	default:
		return fixedError("history_ai_host_local_or_external_qualification_rejected")
	}
}

// This read-only entry hosts the borrowed read transaction and sources.
// It does not import qualification, write evidence, migrate or send messages.
func runAIHostCLI(ctx context.Context, args []string) (r aiHostReport, result error) {
	r = aiHostReport{Protocol: "qs-compatibility-ai-host-readonly/v1", DiagnosticOnly: true, ErrorCategory: "none", RequiredAdapters: []string{"independent_production_descriptor_and_bounds_approval", "actual_host_process_memory_cpu_and_scan_budget", "original_exec_unknown_keeps_mutation_blocked", "whole_writer_fence_for_any_mutation", "historical_evidence_write_authority", "post_write_independent_readback", "backup_restore_acceptance_purge"}}
	flags, e := parseAIHostFlags(args)
	if e != nil {
		return r, e
	}
	r.Mode, r.SourceSHA, r.OperationID, r.ActualRunID = flags["ai-host-mode"], sourceSHA, flags["operation"], flags["run"]
	if flags["original-source-sha"] != "" {
		r.SourceSHA, r.ToolSourceSHA = flags["original-source-sha"], sourceSHA
	}
	r.ExternalRunID, _ = aiHostRun(r.ActualRunID)
	r.RequestSHA256, r.DescriptorSHA256 = flags["request-sha256"], flags["ai-input-sha256"]
	lock, e := lockAIHostOperation(flags["operation-directory"])
	if e != nil {
		return r, e
	}
	var assets []*aiHostPrivateAsset
	var a *approvedInputs
	var db *historyDatabase
	outputOwned := false
	lockBefore, _ := lock.Stat()
	defer func() {
		// All close failures change both private and stdout diagnostic before success.
		if db != nil && db.close() != nil && result == nil {
			result = fixedError("history_connection_close_failed")
		}
		if a != nil && a.close() != nil && result == nil {
			result = fixedError("history_private_close_failed")
		}
		for _, asset := range assets {
			if asset.file.Close() != nil && result == nil {
				result = fixedError("history_ai_host_private_close_failed")
			}
		}
		lockAfter, le := lock.Stat()
		lockNamed, ln := os.Lstat(filepath.Join(flags["operation-directory"], "operation.lock"))
		if (le != nil || ln != nil || !aiHostSame(lockBefore, lockAfter) || !aiHostSame(lockBefore, lockNamed)) && result == nil {
			result = fixedError("history_ai_host_operation_lock_changed")
		}
		if lock.Close() != nil && result == nil {
			result = fixedError("history_ai_host_operation_close_failed")
		}
		r.DiagnosticReadComplete = result == nil
		r.ErrorCategory = safeCategory(result)
		raw, me := json.Marshal(r)
		if outputOwned && (me != nil || aiHostWriteFile(flags["output"], "ai-host.readiness.json", append(raw, '\n')) != nil) {
			if result == nil {
				result = fixedError("history_ai_host_private_output_rejected")
			}
			r.DiagnosticReadComplete = false
			r.ErrorCategory = safeCategory(result)
		}
	}()
	// Never adopt/overwrite output from another attempt or interrupted producer.
	if os.Mkdir(flags["output"], 0700) != nil {
		return r, fixedError("history_ai_host_output_exists_or_unavailable")
	}
	outputOwned = true
	input, e := readAIHostAsset(fileBinding{flags["ai-input"], flags["ai-input-sha256"]}, maximumJSONBytes)
	if e != nil {
		return r, e
	}
	assets = append(assets, input)
	var v aiHostDescriptor
	if strictDecode(input.raw, &v) != nil || validateAIHostDescriptor(v, flags) != nil {
		return r, fixedError("history_ai_host_descriptor_rejected")
	}
	r.ExpectedRuntimeBindingSHA256 = v.RuntimeBindingSHA256
	packets := map[string][]byte{}
	if v.Mode == "verify" {
		for _, item := range []struct {
			name string
			b    *fileBinding
			max  uint64
		}{{"ai", v.AIBounds, 4 << 20}, {"peer", v.PeerBounds, 4 << 20}, {"protection", v.Protection, maximumJSONBytes}} {
			asset, e := readAIHostAsset(*item.b, item.max)
			if e != nil {
				return r, e
			}
			assets = append(assets, asset)
			packets[item.name] = asset.raw
		}
	}
	a, e = loadInputsForSource(ctx, flags["request"], flags["request-sha256"], flags["operation"], flags["run"], r.SourceSHA)
	if e != nil {
		return r, e
	}
	for _, path := range []string{a.request.InventoryRequest.Path, a.request.InventoryReport.Path} {
		if !aiHostUnder(flags["operation-directory"], path) {
			return r, fixedError("history_ai_host_operation_directory_rejected")
		}
	}
	for _, asset := range a.request.Assets {
		if !aiHostUnder(flags["operation-directory"], asset.Path) {
			return r, fixedError("history_ai_host_operation_directory_rejected")
		}
	}
	peer, e := aiHostPeerConnection()
	if e != nil {
		return r, e
	}
	db, e = openDatabases(ctx, a)
	if e != nil {
		return r, e
	}
	result = db.epoch(ctx, func(scope context.Context) error {
		if a.verifyFullFiles(scope) != nil || a.rewind() != nil {
			return fixedError("history_asset_hash_changed")
		}
		c, e := retirement.PrepareHistoricalCoordinator(scope, retirement.HistoricalCoordinatorBinding{SourceSHA: a.request.SourceSHA, OperationID: a.request.OperationID}, a.copies(), retirement.DefaultHistoricalCoordinatorLimits())
		if e != nil {
			return fixedError("history_source_authentication_failed")
		}
		current, e := retirement.PrepareSQLResponsibilitySnapshot(scope, a.inventory.Identities["mysql"], sqlevaluation.DefaultSQLResponsibilityLimits())
		if e != nil {
			return fixedError("history_global_sql_scan_failed")
		}
		if a.rewind() != nil {
			return fixedError("history_asset_read_failed")
		}
		local, e := c.PrepareAIReadOnlyResolver(scope, current, a.inventory.Migrations["mysql"], a.copies()[1:3])
		if e != nil {
			return fixedError("history_ai_readonly_prepare_failed")
		}
		for _, asset := range assets {
			if asset.recheck() != nil {
				return fixedError("history_ai_host_private_input_changed")
			}
		}
		if e = retirement.RequireAIExternalExecQuiescence(scope, retirement.AIExternalExecQuiescenceInput{OperationDirectory: flags["operation-directory"], SourceSHA: a.request.SourceSHA, OperationID: a.request.OperationID, RuntimeSourceSHA: v.RuntimeSourceSHA, ImageID: v.ImageID, ContainerID: v.ContainerID, SudoDocker: true}); e != nil {
			return aiHostCategory(e)
		}
		if v.Mode == "bounds" {
			o, e := c.ObserveAIExternalBounds(scope, local, retirement.AIExternalBoundsInput{OperationDirectory: flags["operation-directory"], AssetsDirectory: v.AssetsDirectory, RunID: r.ExternalRunID, RuntimeSourceSHA: v.RuntimeSourceSHA, ImageID: v.ImageID, ContainerID: v.ContainerID, ApprovedAIRuntimeBindingSHA256: v.RuntimeBindingSHA256, ExpectedAIIdentityHash: v.ExpectedAIIdentityHash, ExpectedAIHead: v.ExpectedAIHead, PeerConnection: peer, SudoDocker: true})
			if e != nil {
				return aiHostCategory(e)
			}
			ai, peer, e := o.PrivatePackets()
			if e != nil {
				return aiHostCategory(e)
			}
			if aiHostWriteFile(flags["output"], "ai.bounds.json", ai) != nil || aiHostWriteFile(flags["output"], "peer.bounds.json", peer) != nil {
				return fixedError("history_ai_host_private_output_rejected")
			}
			summary := o.Summary()
			r.Bounds = &summary
			r.RuntimeBindingSHA256 = summary.RuntimeBindingSHA256
		} else {
			reverse, e := retirement.PrepareAIReverseSnapshot(scope, current, a.inventory.Migrations["mysql"], retirement.DefaultAIReverseLimits())
			if e != nil {
				return fixedError("history_ai_reverse_scan_failed")
			}
			if a.rewind() != nil {
				return fixedError("history_asset_read_failed")
			}
			if c.BindAIReverseSourceScope(scope, reverse, a.copies()) != nil {
				return fixedError("history_ai_reverse_source_binding_failed")
			}
			q, e := c.PrepareAIExternalExecution(scope, local, reverse, retirement.AIExternalExecutionInput{OperationDirectory: flags["operation-directory"], AssetsDirectory: v.AssetsDirectory, RunID: r.ExternalRunID, RuntimeSourceSHA: v.RuntimeSourceSHA, ImageID: v.ImageID, ContainerID: v.ContainerID, ApprovedAIRuntimeBindingSHA256: v.RuntimeBindingSHA256, AIBounds: packets["ai"], PeerBounds: packets["peer"], ApprovedAIBoundsSHA256: v.AIBounds.SHA256, ApprovedPeerBoundsSHA256: v.PeerBounds.SHA256, ProtectionJSON: packets["protection"], PeerConnection: peer, SudoDocker: true})
			if e != nil {
				return aiHostCategory(e)
			}
			summary := q.Summary()
			r.Verification = &aiHostVerificationSummary{Scope: summary.Scope, Originals: summary.Originals, FactsSHA256: summary.FactsSHA256, IndependentEpochs: summary.IndependentEpochs, ExternalDatabaseFactsObserved: summary.ExternalDatabaseFactsObserved}
			r.RuntimeBindingSHA256 = v.RuntimeBindingSHA256
			// q stays opaque and in this live scope only. Saving the summary cannot
			// reproduce it for a different coordinator or for retirement writes.
			if reverse.ValidateBorrowedSnapshot(scope) != nil {
				return fixedError("history_ai_reverse_snapshot_changed_before_close")
			}
		}
		for _, asset := range assets {
			if asset.recheck() != nil {
				return fixedError("history_ai_host_private_input_changed")
			}
		}
		if current.ValidateBorrowedSnapshot(scope) != nil || a.verifyFullFiles(scope) != nil {
			return fixedError("history_epoch_changed_before_close")
		}
		return nil
	})
	return r, result
}
