package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"syscall"
	"time"

	sqlevaluation "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	retirement "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirement"
	hostmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readconcern"
	"go.mongodb.org/mongo-driver/mongo/readpref"
	"golang.org/x/sys/unix"
)

// The explicit CLI owns this actual host implementation for limited
// event/old-command evidence persistence,
// not a production authority. Actual external facts are obtained only by the
// fixed producer; current mapped pending/provider work is never completed here.
type preparedWriteDiagnostic struct {
	Protocol, SourceSHA, OperationID, RunID, CommitState, MongoCommitRequirement                                                   string
	AIOriginalCommands, AISourceReferences                                                                                         uint64
	PreparedPages, ReadBackPages, EventReferences                                                                                  uint64
	ActualSQLCommitResponse, ActualMongoCommitResponse                                                                             bool
	LimitedEventPersistenceObserved                                                                                                bool
	AICommandPersistenceComplete, FullExternalAIClosureVerified, WriterFenceProven, CASComplete, DropReady, MutationBackendEnabled bool
	Required                                                                                                                       []string
	MaterialManifestSHA256                                                                                                         string
}
type historyWriteJournal struct {
	path     string
	dir      *os.File
	dev, ino uint64
	sequence uint64
	binding  retirement.HistoricalCoordinatorBinding
	run      string
	unknown  bool
	created  map[string]os.FileInfo
}

type historyTemporaryMaterial struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
	UID    uint32 `json:"uid"`
	GID    uint32 `json:"gid"`
	Device uint64 `json:"device"`
	Inode  uint64 `json:"inode"`
	Mode   uint32 `json:"mode"`
}
type historyTemporaryMaterialManifest struct {
	Version         int                        `json:"version"`
	SourceSHA       string                     `json:"source_sha"`
	ToolSourceSHA   string                     `json:"tool_source_sha"`
	OperationID     string                     `json:"operation_id"`
	RunID           string                     `json:"run_id"`
	MaxSpoolBytes   int64                      `json:"max_spool_bytes"`
	JournalSequence uint64                     `json:"journal_sequence"`
	Files           []historyTemporaryMaterial `json:"files"`
}
type historyWriteJournalRecord struct {
	Version        int                                           `json:"version"`
	SourceSHA      string                                        `json:"source_sha"`
	ToolSourceSHA  string                                        `json:"tool_source_sha,omitempty"`
	OperationID    string                                        `json:"operation_id"`
	RunID          string                                        `json:"run_id"`
	Sequence       uint64                                        `json:"sequence"`
	Stage          string                                        `json:"stage"`
	PagePosition   int                                           `json:"page_position"`
	References     uint64                                        `json:"references"`
	Observation    *retirement.HistoricalCASSpoolPageObservation `json:"observation,omitempty"`
	Complete       bool                                          `json:"complete"`
	ExecutionReady bool                                          `json:"execution_ready"`
	CASAuthorized  bool                                          `json:"cas_authorized"`
	DropReady      bool                                          `json:"drop_ready"`
}

func newHistoryWriteJournal(dir string, a *approvedInputs) (*historyWriteJournal, error) {
	if a == nil || !sourcePattern.MatchString(a.request.SourceSHA) || !runPattern.MatchString(a.request.OperationID) || !runPattern.MatchString(a.request.RunID) {
		return nil, fixedError("history_write_input_rejected")
	}
	if !filepath.IsAbs(dir) || filepath.Clean(dir) != dir {
		return nil, fixedError("history_write_private_file_rejected")
	}
	parentPath := filepath.Dir(dir)
	parent, e := os.Lstat(parentPath)
	if e != nil {
		return nil, fixedError("history_write_private_file_rejected")
	}
	pst, ok := parent.Sys().(*syscall.Stat_t)
	if !ok || !parent.IsDir() || parent.Mode()&os.ModeSymlink != 0 || parent.Mode().Perm() != 0o700 || pst.Uid != uint32(os.Geteuid()) {
		return nil, fixedError("history_write_private_file_rejected")
	}
	pfd, e := unix.Open(parentPath, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if e != nil {
		return nil, fixedError("history_write_private_file_rejected")
	}
	parentFD := os.NewFile(uintptr(pfd), parentPath)
	pinned, pe := parentFD.Stat()
	if pe != nil || !os.SameFile(parent, pinned) {
		_ = parentFD.Close()
		return nil, fixedError("history_write_private_file_rejected")
	}
	// Both directory creation and file writes are durable before any statement;
	// an existing/partial directory is never adopted or automatically removed.
	if unix.Mkdirat(pfd, filepath.Base(dir), 0o700) != nil {
		_ = parentFD.Close()
		return nil, fixedError("history_write_output_exists_or_unknown")
	}
	fd, e := unix.Openat(pfd, filepath.Base(dir), unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if e != nil {
		_ = parentFD.Close()
		return nil, fixedError("history_write_private_file_rejected")
	}
	f := os.NewFile(uintptr(fd), dir)
	info, e := f.Stat()
	if e != nil {
		_ = f.Close()
		_ = parentFD.Close()
		return nil, fixedError("history_write_private_file_rejected")
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	syncError, closeError := parentFD.Sync(), parentFD.Close()
	if !ok || !info.IsDir() || info.Mode().Perm() != 0o700 || st.Uid != uint32(os.Geteuid()) || syncError != nil || closeError != nil {
		_ = f.Close()
		return nil, fixedError("history_write_private_file_rejected")
	}
	visible, ve := os.Lstat(dir)
	if ve != nil || !os.SameFile(info, visible) {
		_ = f.Close()
		return nil, fixedError("history_write_private_file_rejected")
	}
	return &historyWriteJournal{path: dir, dir: f, dev: uint64(st.Dev), ino: uint64(st.Ino), binding: retirement.HistoricalCoordinatorBinding{SourceSHA: a.request.SourceSHA, OperationID: a.request.OperationID}, run: a.request.RunID, created: map[string]os.FileInfo{}}, nil
}
func (j *historyWriteJournal) valid() bool {
	if j == nil || j.dir == nil || j.unknown {
		return false
	}
	info, e := j.dir.Stat()
	if e != nil {
		return false
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	pathInfo, pe := os.Lstat(j.path)
	if pe != nil || !os.SameFile(info, pathInfo) {
		return false
	}
	return ok && info.IsDir() && info.Mode().Perm() == 0o700 && st.Uid == uint32(os.Geteuid()) && uint64(st.Dev) == j.dev && uint64(st.Ino) == j.ino
}
func (j *historyWriteJournal) create(name string) (*os.File, error) {
	if !j.valid() {
		return nil, fixedError("history_write_journal_unknown")
	}
	fd, e := unix.Openat(int(j.dir.Fd()), name, unix.O_RDWR|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0o600)
	if e != nil {
		return nil, fixedError("history_write_private_file_rejected")
	}
	f := os.NewFile(uintptr(fd), name)
	info, err := f.Stat()
	if err != nil || j.created[name] != nil {
		_ = f.Close()
		j.unknown = true
		return nil, fixedError("history_write_private_file_rejected")
	}
	j.created[name] = info
	return f, nil
}

func historyMaterialSame(a, b os.FileInfo) bool {
	if !aiHostSame(a, b) {
		return false
	}
	x, y := reflect.ValueOf(a.Sys()).Elem(), reflect.ValueOf(b.Sys()).Elem()
	for _, field := range []string{"Ctim", "Ctimespec"} {
		if x.FieldByName(field).IsValid() {
			return reflect.DeepEqual(x.FieldByName(field).Interface(), y.FieldByName(field).Interface())
		}
	}
	return false
}

// Called by the original producer only after both spool writer Close results.
// The exact names come from original create calls/sequence, never directory
// adoption. This descriptor is private metadata, not a retirement capability.
func (j *historyWriteJournal) snapshotMaterials(ctx context.Context) (string, error) {
	if ctx == nil || ctx.Err() != nil || !j.valid() || j.sequence > 1<<20 {
		return "", fixedError("history_write_material_binding_rejected")
	}
	var names []string
	version := 1
	inputNames := historyInitialInputNames()
	for _, name := range inputNames {
		if j.created[name] != nil {
			names = append(names, inputNames[:]...)
			break
		}
	}
	if len(names) != 0 || j.created[historyInputOwnerSQLName] != nil {
		if len(names) != len(inputNames) || j.created[historyInputOwnerSQLName] == nil || j.created["prepared-mongo-private.bin"] != nil || j.created["prepared-sql-private.bin"] != nil {
			return "", fixedError("history_write_material_binding_rejected")
		}
		names = append(names, historyInputOwnerSQLName)
		version = 2
	} else {
		if j.created["prepared-mongo-private.bin"] == nil || j.created["prepared-sql-private.bin"] == nil {
			return "", fixedError("history_write_material_binding_rejected")
		}
		names = []string{"prepared-mongo-private.bin", "prepared-sql-private.bin"}
	}
	for _, name := range names {
		if j.created[name] == nil {
			return "", fixedError("history_write_material_binding_rejected")
		}
	}
	largeFiles := len(names)
	if largeFiles == 0 || len(j.created) != largeFiles+int(j.sequence) {
		return "", fixedError("history_write_material_binding_rejected")
	}
	for n := uint64(1); n <= j.sequence; n++ {
		names = append(names, "journal-"+strconv.FormatUint(n, 10)+".json")
	}
	entries, e := os.ReadDir(j.path)
	if e != nil || len(entries) != len(names) {
		return "", fixedError("history_write_material_binding_rejected")
	}
	for _, entry := range entries {
		if j.created[entry.Name()] == nil || entry.IsDir() {
			return "", fixedError("history_write_material_binding_rejected")
		}
	}
	m := historyTemporaryMaterialManifest{Version: version, SourceSHA: j.binding.SourceSHA, ToolSourceSHA: sourceSHA, OperationID: j.binding.OperationID, RunID: j.run, MaxSpoolBytes: 16 << 30, JournalSequence: j.sequence}
	for index, name := range names {
		if ctx.Err() != nil || !j.valid() {
			return "", fixedError("history_write_material_binding_rejected")
		}
		fd, e := unix.Openat(int(j.dir.Fd()), name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
		if e != nil {
			return "", fixedError("history_write_material_binding_rejected")
		}
		f := os.NewFile(uintptr(fd), name)
		before, be := f.Stat()
		named, ne := os.Lstat(filepath.Join(j.path, name))
		original := j.created[name]
		limit := int64(64 << 10)
		if index < largeFiles {
			limit = 16 << 30
		}
		if be != nil || ne != nil || original == nil || !os.SameFile(original, before) || !aiHostRegular(before, uint64(limit)) || !historyMaterialSame(before, named) {
			_ = f.Close()
			return "", fixedError("history_write_material_binding_rejected")
		}
		ost, ook := original.Sys().(*syscall.Stat_t)
		st, sok := before.Sys().(*syscall.Stat_t)
		if !ook || !sok || ost.Uid != st.Uid || ost.Gid != st.Gid || original.Mode() != before.Mode() {
			_ = f.Close()
			return "", fixedError("history_write_material_binding_rejected")
		}
		h := sha256.New()
		n, re := io.Copy(h, io.LimitReader(f, before.Size()+1))
		after, ae := f.Stat()
		named, ne = os.Lstat(filepath.Join(j.path, name))
		ce := f.Close()
		if ctx.Err() != nil || re != nil || ae != nil || ne != nil || ce != nil || n != before.Size() || !historyMaterialSame(before, after) || !historyMaterialSame(before, named) {
			return "", fixedError("history_write_material_binding_rejected")
		}
		m.Files = append(m.Files, historyTemporaryMaterial{name, hex.EncodeToString(h.Sum(nil)), n, st.Uid, st.Gid, uint64(st.Dev), uint64(st.Ino), uint32(before.Mode().Perm())})
	}
	raw, e := json.Marshal(m)
	if e != nil || len(raw) > 64<<20 {
		return "", fixedError("history_write_material_binding_rejected")
	}
	f, e := j.create("history.materials.private.json")
	if e != nil {
		return "", e
	}
	n, we := f.Write(raw)
	se, ce := f.Sync(), f.Close()
	if we != nil || n != len(raw) || se != nil || ce != nil || j.dir.Sync() != nil {
		j.unknown = true
		return "", fixedError("history_write_material_binding_rejected")
	}
	return rawHash(raw), nil
}
func (j *historyWriteJournal) record(ctx context.Context, stage string, position int, refs uint64, observation *retirement.HistoricalCASSpoolPageObservation) error {
	if ctx == nil || ctx.Err() != nil || !j.valid() {
		return fixedError("history_write_journal_unknown")
	}
	switch stage {
	case "initial_input_epoch_frozen", "initial_inputs_matched", "prepared", "write_epoch_started", "page_statement_applied", "all_statements_applied", "sql_commit_intent", "sql_commit_success", "sql_commit_unknown", "actual_origin_readback_matched", "mongo_commit_not_required", "mongo_commit_intent", "mongo_commit_success", "mongo_commit_unknown", "mongo_abort_failed", "sql_rollback_failed", "fresh_page_verified", "limited_event_readback_finished", "ai_statements_applied", "ai_independent_readback_finished":
	default:
		return fixedError("history_write_journal_unknown")
	}
	rec := historyWriteJournalRecord{Version: 1, SourceSHA: j.binding.SourceSHA, ToolSourceSHA: sourceSHA, OperationID: j.binding.OperationID, RunID: j.run, Sequence: j.sequence + 1, Stage: stage, PagePosition: position, References: refs, Observation: observation}
	raw, e := json.Marshal(rec)
	if e != nil || len(raw) > 64<<10 {
		return fixedError("history_write_journal_unknown")
	}
	name := "journal-" + strconv.FormatUint(rec.Sequence, 10) + ".json"
	f, e := j.create(name)
	if e != nil {
		return e
	}
	n, we := f.Write(raw)
	se := f.Sync()
	ce := f.Close()
	if we != nil || n != len(raw) || se != nil || ce != nil || j.dir.Sync() != nil {
		j.unknown = true
		return fixedError("history_write_journal_unknown")
	}
	j.sequence++
	return nil
}

func executeHistoricalEvidenceWrite(ctx context.Context, a *approvedInputs, d *historyDatabase, outputDir string, external ...retirement.AIExternalExecutionInput) (report preparedWriteDiagnostic, result error) {
	report = preparedWriteDiagnostic{Protocol: "limited-historical-evidence-host/v1", CommitState: "not_attempted", MongoCommitRequirement: "undetermined", Required: []string{"independent_production_source_approval", "whole_writer_and_historical_rerun_fence", "production_process_and_transaction_budget", "actual_external_qs_ai_facts_for_unmapped_original_commands", "complete_original_command_evidence_persistence_and_readback", "final_expected_global_and_absent_range_fence", "prewindow_actual_isolated_restore", "maintenance_acceptance_and_purge"}}
	if ctx == nil || ctx.Err() != nil || a == nil || d == nil || len(external) > 1 {
		return report, fixedError("history_write_input_rejected")
	}
	report.SourceSHA, report.OperationID, report.RunID = a.request.SourceSHA, a.request.OperationID, a.request.RunID
	journal, err := newHistoryWriteJournal(outputDir, a)
	if err != nil {
		return report, err
	}
	defer func() {
		if result == nil {
			report.MaterialManifestSHA256, result = journal.snapshotMaterials(ctx)
		}
		if journal.dir.Close() != nil && result == nil {
			result = fixedError("history_write_private_close_failed")
		}
		if result != nil {
			report.MaterialManifestSHA256 = ""
		}
	}()
	// These are the only full initial captures. They use two actual ended
	// RR-RO/snapshot-only scopes and retain input/owner recipes, never an old
	// consumed global page or write capability. No buildEpoch path runs here.
	inputs, err := captureHistoryInitialInputs(ctx, a, d, journal)
	if err != nil {
		return report, err
	}
	defer func() {
		if e := inputs.close(); result == nil && e != nil {
			result = e
		}
	}()
	if inputs.components.ValidateInputSources(ctx, inputs.sources) != nil || inputs.ai.ValidateFrozen(ctx) != nil {
		return report, fixedError("history_write_initial_inputs_changed")
	}
	components := inputs.components.Components()
	if a.copies()[0].Expected.Records+a.copies()[1].Expected.Records+a.copies()[2].Expected.Records+a.copies()[3].Expected.Records == 0 {
		return report, fixedError("history_write_no_original_statements")
	}
	// The fixed external producer runs outside the twenty-second event scopes.
	// Its returned Q and original input are process-local capabilities, never
	// a saved successful receipt. AI evidence is committed/read back first,
	// since event evidence would otherwise alter its actual owner anchors.
	if len(external) != 1 {
		return report, fixedError("history_ai_external_original_facts_required")
	}
	aiInput := external[0]
	aiInput.Binding = retirement.HistoricalCoordinatorBinding{SourceSHA: a.request.SourceSHA, OperationID: a.request.OperationID}
	canonicalRun, runErr := aiHostRun(a.request.RunID)
	if runErr != nil || aiInput.RunID != canonicalRun {
		return report, fixedError("history_ai_external_execution_input_required")
	}
	if retirement.RequireAIExternalExecQuiescence(ctx, retirement.AIExternalExecQuiescenceInput{OperationDirectory: aiInput.OperationDirectory, SourceSHA: a.request.SourceSHA, OperationID: a.request.OperationID, RuntimeSourceSHA: aiInput.RuntimeSourceSHA, ImageID: aiInput.ImageID, ContainerID: aiInput.ContainerID, SudoDocker: aiInput.SudoDocker}) != nil {
		return report, fixedError("history_ai_external_exec_quiescence_unproven")
	}
	var qualifiedAI *retirement.AIExternalExecutionQualification
	var aiBatch *retirement.AICommandPersistenceBatch
	if e := d.epoch(ctx, func(scope context.Context) error {
		var err error
		qualifiedAI, err = retirement.PrepareHistoricalAIExternalExecution(scope, inputs.sources, inputs.ai, aiInput)
		if err != nil {
			return fixedError("history_ai_external_original_facts_failed")
		}
		aiBatch, err = retirement.PrepareHistoricalAICommandPersistenceBatch(scope, inputs.sources, inputs.ai, qualifiedAI)
		if err != nil {
			return fixedError("history_ai_original_persistence_prepare_failed")
		}
		return nil
	}); e != nil {
		return report, e
	}
	aiCounts, aiCommit, e := d.writeHistoricalAI(ctx, aiBatch, journal)
	report.CommitState, report.MongoCommitRequirement = aiCommit.CommitState, aiCommit.MongoCommitRequirement
	report.ActualSQLCommitResponse = aiCommit.ActualSQLCommitResponse
	report.AIOriginalCommands, report.AISourceReferences = aiCounts.OriginalCommands, aiCounts.OriginalSourceReferences
	if e != nil {
		return report, e
	}
	if e = d.epoch(ctx, func(scope context.Context) error {
		observation, err := aiBatch.VerifyHistoricalReadback(scope, inputs.sources, inputs.ai)
		if err != nil || !observation.IndependentReadbackMatched || observation.OriginalCommands != aiCounts.OriginalCommands || observation.OriginalSourceReferences != aiCounts.OriginalSourceReferences {
			return fixedError("history_ai_original_independent_readback_failed")
		}
		return journal.record(scope, "ai_independent_readback_finished", -1, observation.OriginalSourceReferences, nil)
	}); e != nil {
		return report, e
	}
	report.AICommandPersistenceComplete = true
	expected := a.copies()[0].Expected.Records + a.copies()[3].Expected.Records
	var mongoComponents uint64
	for position, component := range components {
		// The aggregate hashes are checked before and after this loop. Each
		// actual component revalidates the borrowed original FD and selected
		// frame hashes; rescanning every source for every component would turn
		// bounded work back into a repeated full-source traversal.
		if ctx.Err() != nil {
			return report, fixedError("history_write_budget_exhausted")
		}
		// Qualification and the first effect share this short fresh native RW
		// scope. The actual adapter locks the original full-row SQL baseline;
		// Mongo forces writes to every selected current dependency before CAS.
		statements, mongoStatement, refs, componentReport, e := d.writeHistoricalComponent(ctx, a, inputs, component, position, journal, qualifiedAI, aiBatch)
		report.ActualSQLCommitResponse = report.ActualSQLCommitResponse || componentReport.ActualSQLCommitResponse
		report.ActualMongoCommitResponse = report.ActualMongoCommitResponse || componentReport.ActualMongoCommitResponse
		if componentReport.MongoCommitRequirement == "required" {
			report.MongoCommitRequirement = "required"
		}
		if componentReport.CommitState != "not_attempted" {
			report.CommitState = componentReport.CommitState
		}
		if e != nil {
			return report, e
		}
		report.PreparedPages++
		report.EventReferences += refs
		if refs > 0 {
			mongoComponents++
		}
		if e = d.epoch(ctx, func(scope context.Context) error {
			for _, statement := range statements {
				if _, e := statement.VerifyIndependentPersisted(scope, 20*time.Second); e != nil {
					return fixedError("history_write_independent_readback_failed")
				}
			}
			if mongoStatement != nil && mongoStatement.VerifyIndependentPersisted(scope, d.mongo, 20*time.Second) != nil {
				return fixedError("history_write_independent_readback_failed")
			}
			return journal.record(scope, "fresh_page_verified", position, refs, nil)
		}); e != nil {
			return report, e
		}
		report.ReadBackPages++
	}
	if report.EventReferences != expected || report.ReadBackPages != report.PreparedPages || inputs.sources.ValidateFrozen(ctx) != nil || inputs.ai.ValidateFrozen(ctx) != nil || a.verifyFullFiles(ctx) != nil {
		return report, fixedError("history_write_independent_readback_failed")
	}
	if err = journal.record(ctx, "limited_event_readback_finished", -1, report.EventReferences, nil); err != nil {
		return report, err
	}
	report.ActualSQLCommitResponse = true
	report.ActualMongoCommitResponse = mongoComponents > 0
	report.MongoCommitRequirement = "not_required"
	report.CommitState = "sql_committed_mongo_not_required"
	if mongoComponents > 0 {
		report.MongoCommitRequirement, report.CommitState = "required", "both_responses_success_non_atomic"
	}
	report.LimitedEventPersistenceObserved = expected > 0
	return report, nil
}

// A known SQL response and a different real readback are mandatory before any
// event evidence changes owner rows. Unknown Record/Commit is never replayed.
func (d *historyDatabase) writeHistoricalAI(ctx context.Context, batch *retirement.AICommandPersistenceBatch, journal *historyWriteJournal) (counts retirement.AICommandPersistenceSummary, report preparedWriteDiagnostic, result error) {
	report.CommitState, report.MongoCommitRequirement = "not_attempted", "not_required"
	if ctx == nil || ctx.Err() != nil || d == nil || d.sql == nil || batch == nil || journal == nil || d.validateNamespaceAnchor(ctx) != nil {
		return counts, report, fixedError("history_write_host_epoch_rejected")
	}
	bounded, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	tx := d.sql.WithContext(bounded).Begin(&sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: false})
	if tx.Error != nil {
		return counts, report, fixedError("history_write_host_epoch_rejected")
	}
	committed := false
	defer func() {
		if !committed {
			if e := tx.Rollback().Error; e != nil && !errors.Is(e, sql.ErrTxDone) {
				cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
				_ = journal.record(cleanup, "sql_rollback_failed", -1, counts.OriginalSourceReferences, nil)
				stop()
				journal.unknown = true
				if result == nil {
					result = fixedError("history_write_cleanup_unknown")
				}
			}
		}
	}()
	paired := hostmysql.WithTx(bounded, tx)
	counts, err := batch.Record(paired, tx)
	if err != nil || !counts.InTransactionWritten && (counts.OriginalCommands != 0 || counts.OriginalSourceReferences != 0) {
		return counts, report, fixedError("history_ai_original_persistence_statement_failed")
	}
	if journal.record(paired, "ai_statements_applied", -1, counts.OriginalSourceReferences, nil) != nil || journal.record(paired, "sql_commit_intent", -1, counts.OriginalSourceReferences, nil) != nil {
		return counts, report, fixedError("history_write_journal_unknown")
	}
	committed = true // Commit ATTEMPT poisons this original batch even if unknown.
	if tx.Commit().Error != nil {
		report.CommitState = "sql_unknown_mongo_not_attempted"
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		_ = journal.record(cleanup, "sql_commit_unknown", -1, counts.OriginalSourceReferences, nil)
		stop()
		journal.unknown = true
		return counts, report, fixedError("history_write_commit_unknown")
	}
	report.ActualSQLCommitResponse = true
	report.CommitState = "sql_committed_mongo_not_required"
	if journal.record(ctx, "sql_commit_success", -1, counts.OriginalSourceReferences, nil) != nil {
		report.CommitState = "sql_response_success_journal_unknown"
		journal.unknown = true
		return counts, report, fixedError("history_write_journal_unknown")
	}
	if d.validateNamespaceAnchor(ctx) != nil {
		return counts, report, fixedError("history_database_binding_rejected")
	}
	return counts, report, nil
}

// One component has one fresh host-owned native RW scope, one statement phase
// and at most one commit attempt per store. Its returned opaque statements are
// read back only after both known commit responses and this scope has ended.
func (d *historyDatabase) writeHistoricalComponent(ctx context.Context, a *approvedInputs, inputs *historyInitialInputs, component *retirement.HistoricalCASComponent, position int, journal *historyWriteJournal, qualifiedAI *retirement.AIExternalExecutionQualification, aiBatch *retirement.AICommandPersistenceBatch) (statements []*sqlevaluation.SQLHistoricalComponentStatement, mongoStatement *retirement.MongoHistoricalComponentStatement, refs uint64, report preparedWriteDiagnostic, result error) {
	report.CommitState, report.MongoCommitRequirement = "not_attempted", "not_required"
	if d == nil || d.sql == nil || d.mongo == nil || d.mongoClient == nil || a == nil || inputs == nil || component == nil || journal == nil || ctx == nil || ctx.Err() != nil || qualifiedAI == nil || aiBatch == nil || d.validateNamespaceAnchor(ctx) != nil {
		result = fixedError("history_write_host_epoch_rejected")
		return
	}
	bounded, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	session, err := d.mongoClient.StartSession()
	if err != nil {
		result = fixedError("history_write_host_epoch_rejected")
		return
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		session.EndSession(cleanup)
		stop()
	}()
	if session.StartTransaction(options.Transaction().SetReadConcern(readconcern.Snapshot()).SetReadPreference(readpref.Primary())) != nil {
		result = fixedError("history_write_host_epoch_rejected")
		return
	}
	tx := d.sql.WithContext(bounded).Begin(&sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: false})
	if tx.Error != nil {
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		if session.AbortTransaction(cleanup) != nil {
			journal.unknown = true
		}
		stop()
		result = fixedError("history_write_host_epoch_rejected")
		return
	}
	sqlCommitAttempt, mongoCommitAttempt := false, false
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		if !sqlCommitAttempt {
			if e := tx.Rollback().Error; e != nil && !errors.Is(e, sql.ErrTxDone) {
				_ = journal.record(cleanup, "sql_rollback_failed", position, refs, nil)
				journal.unknown = true
				if result == nil {
					result = fixedError("history_write_cleanup_unknown")
				}
			}
		}
		if !mongoCommitAttempt {
			if session.AbortTransaction(cleanup) != nil {
				_ = journal.record(cleanup, "mongo_abort_failed", position, refs, nil)
				journal.unknown = true
				if result == nil {
					result = fixedError("history_write_cleanup_unknown")
				}
			}
		}
		if e := d.validateNamespaceAnchor(cleanup); result == nil && e != nil {
			result = e
		}
	}()
	paired := mongo.NewSessionContext(hostmysql.WithTx(bounded, tx), session)
	if journal.record(paired, "write_epoch_started", position, 0, nil) != nil {
		result = fixedError("history_write_journal_unknown")
		return
	}
	observed, e := retirement.PrepareHistoricalComponentObservation(paired, component, inputs.sources, inputs.ai, d.mongo, 20*time.Second, true)
	if e != nil {
		result = fixedError("history_write_qualification_failed")
		return
	}
	// This factory consumes actual native source/business/AI composition.
	// It rejects unclosed related obligations instead of accepting summaries.
	statements, mongoStatement, refs, e = retirement.ApplyQualifiedHistoricalComponent(paired, observed, qualifiedAI, aiBatch)
	if e != nil {
		result = fixedError("history_write_qualification_or_statement_failed")
		return
	}
	if refs > 0 {
		report.MongoCommitRequirement = "required"
	}
	if journal.record(paired, "page_statement_applied", position, refs, nil) != nil {
		result = fixedError("history_write_journal_unknown")
		return
	}
	if e = probePreparedMongo(paired, d.mongo, session, report.MongoCommitRequirement); e != nil {
		result = e
		return
	}
	if journal.record(paired, "sql_commit_intent", position, refs, nil) != nil {
		result = fixedError("history_write_journal_unknown")
		return
	}
	sqlCommitAttempt = true
	if tx.Commit().Error != nil {
		report.CommitState = "sql_unknown_mongo_not_attempted"
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		_ = journal.record(cleanup, "sql_commit_unknown", position, refs, nil)
		stop()
		journal.unknown = true
		result = fixedError("history_write_commit_unknown")
		return
	}
	report.ActualSQLCommitResponse = true
	if journal.record(ctx, "sql_commit_success", position, refs, nil) != nil {
		report.CommitState = "sql_response_success_journal_unknown"
		journal.unknown = true
		result = fixedError("history_write_journal_unknown")
		return
	}
	if report.MongoCommitRequirement == "required" {
		if journal.record(ctx, "mongo_commit_intent", position, refs, nil) != nil {
			report.CommitState = "sql_committed_mongo_not_attempted"
			journal.unknown = true
			result = fixedError("history_write_journal_unknown")
			return
		}
		mongoCommitAttempt = true
		report.EventReferences, report.PreparedPages = refs, 1
	}
	result = commitPreparedMongo(bounded, session, journal, &report, position)
	if result != nil {
		journal.unknown = true
	}
	return
}

func commitPreparedMongo(ctx context.Context, session mongo.Session, journal *historyWriteJournal, report *preparedWriteDiagnostic, position int) error {
	if position < -1 || report == nil || !report.ActualSQLCommitResponse || session == nil || journal == nil {
		return fixedError("history_write_host_epoch_rejected")
	}
	if report.MongoCommitRequirement == "not_required" {
		if report.EventReferences != 0 || report.PreparedPages != 0 || report.ActualMongoCommitResponse {
			return fixedError("history_write_host_epoch_rejected")
		}
		report.CommitState = "sql_committed_mongo_not_required"
		return journal.record(ctx, "mongo_commit_not_required", position, report.EventReferences, nil)
	}
	if report.MongoCommitRequirement != "required" {
		return fixedError("history_write_host_epoch_rejected")
	}
	if session.CommitTransaction(ctx) != nil {
		report.CommitState = "sql_committed_mongo_unknown"
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_ = journal.record(cleanup, "mongo_commit_unknown", position, report.EventReferences, nil)
		cancel()
		return fixedError("history_write_commit_unknown")
	}
	report.ActualMongoCommitResponse = true
	report.CommitState = "both_responses_success_non_atomic"
	return journal.record(ctx, "mongo_commit_success", position, report.EventReferences, nil)
}

func requireMongoCommitTransaction(session mongo.Session, requirement string) error {
	if requirement == "not_required" {
		return nil
	}
	if requirement != "required" || session == nil {
		return fixedError("history_write_mongo_transaction_not_started")
	}
	x, ok := session.(mongo.XSession) //nolint:staticcheck // pinned driver actual transaction accessor, already used by the borrowed adapters
	if !ok || x.ClientSession() == nil {
		return fixedError("history_write_mongo_transaction_not_started")
	}
	actual := x.ClientSession()
	if actual.Terminated || !actual.TransactionInProgress() || actual.CurrentRc == nil || actual.CurrentRc.Level != "snapshot" {
		return fixedError("history_write_mongo_transaction_not_started")
	}
	return nil
}

// A local InProgress state can outlive the server transaction. The host probes
// its existing transaction before the first commit; this never opens a session,
// starts a replacement transaction, or repeats a failed commit.
func probePreparedMongo(ctx context.Context, db *mongo.Database, session mongo.Session, requirement string) error {
	if requirement == "not_required" {
		return nil
	}
	if requirement != "required" || ctx == nil || ctx.Err() != nil || db == nil || session == nil || session.Client() != db.Client() {
		return fixedError("history_write_mongo_server_probe_rejected")
	}
	x, ok := session.(mongo.XSession) //nolint:staticcheck // inspect the pinned driver's actual transaction identity
	if !ok || x.ClientSession() == nil {
		return fixedError("history_write_mongo_server_probe_rejected")
	}
	actual := x.ClientSession()
	if actual.Terminated || (!actual.TransactionStarting() && !actual.TransactionInProgress()) || actual.CurrentRc == nil || actual.CurrentRc.Level != "snapshot" {
		return fixedError("history_write_mongo_server_probe_rejected")
	}
	q, cancel := context.WithTimeout(mongo.NewSessionContext(ctx, session), 2*time.Second)
	defer cancel()
	// The standard ledger can be empty. ErrNoDocuments still means this exact
	// transaction received a server response; a transport/server failure does not.
	err := db.Collection("rm_outbox").FindOne(q, bson.D{}, options.FindOne().SetProjection(bson.D{{Key: "_id", Value: 1}}).SetMaxTime(2*time.Second)).Err()
	if err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
		return fixedError("history_write_mongo_server_probe_failed")
	}
	return requireMongoCommitTransaction(session, requirement)
}
