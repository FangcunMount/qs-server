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
type preparedWriteEpoch struct {
	facts       *epochResult
	coordinator *retirement.HistoricalCoordinator
	index       *retirement.WholeSourceJointIndex
	catalog     *retirement.SQLCrossStoreResponsibilityCatalog
	global      *retirement.MongoResponsibilitySnapshot
	anchors     []*retirement.WholeSourceJointReplayAnchor
	aiBatch     *retirement.AICommandPersistenceBatch
}
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
	if ctx == nil || ctx.Err() != nil || !j.valid() || j.sequence > 1<<20 || len(j.created) != int(j.sequence)+2 {
		return "", fixedError("history_write_material_binding_rejected")
	}
	names := []string{"prepared-mongo-private.bin", "prepared-sql-private.bin"}
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
	m := historyTemporaryMaterialManifest{Version: 1, SourceSHA: j.binding.SourceSHA, ToolSourceSHA: sourceSHA, OperationID: j.binding.OperationID, RunID: j.run, MaxSpoolBytes: 16 << 30, JournalSequence: j.sequence}
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
		if index < 2 {
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
	case "prepared", "write_epoch_started", "page_statement_applied", "all_statements_applied", "sql_commit_intent", "sql_commit_success", "sql_commit_unknown", "actual_origin_readback_matched", "mongo_commit_not_required", "mongo_commit_intent", "mongo_commit_success", "mongo_commit_unknown", "mongo_abort_failed", "sql_rollback_failed", "fresh_page_verified", "limited_event_readback_finished", "ai_statements_applied", "ai_independent_readback_finished":
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

func buildPreparedWriteEpoch(ctx context.Context, a *approvedInputs, d *historyDatabase, external ...retirement.AIExternalExecutionInput) (*preparedWriteEpoch, error) {
	if a == nil || d == nil || len(external) > 1 {
		return nil, fixedError("history_pipeline_input_rejected")
	}
	if a.verifyFullFiles(ctx) != nil {
		return nil, fixedError("history_asset_hash_changed")
	}
	// A page must leave room for all related original sources of its owners.
	// WholeJoint enforces the exact union and fails closed above its hard cap;
	// this is not a proof of production owner width, transaction TTL or RSS.
	coordinatorLimits := retirement.DefaultHistoricalCoordinatorLimits()
	coordinatorLimits.MaxPageRecords = 16
	c, err := retirement.PrepareHistoricalCoordinator(ctx, retirement.HistoricalCoordinatorBinding{SourceSHA: a.request.SourceSHA, OperationID: a.request.OperationID}, a.copies(), coordinatorLimits)
	if err != nil {
		return nil, fixedError("history_source_authentication_failed")
	}
	if a.rewind() != nil {
		return nil, fixedError("history_asset_read_failed")
	}
	binding, err := c.BindOriginCopies(ctx, a.originCopies(), retirement.DefaultSourceOriginLimits())
	if err != nil {
		return nil, fixedError("history_source_origin_binding_failed")
	}
	if a.rewind() != nil {
		return nil, fixedError("history_asset_read_failed")
	}
	jointLimits := retirement.DefaultWholeSourceJointLimits()
	jointLimits.MaxRelatedSources = 512
	index, err := c.PrepareWholeSourceJointIndex(ctx, a.jointCopies(), jointLimits)
	if err != nil {
		return nil, fixedError("history_complete_source_index_failed")
	}
	current, err := retirement.PrepareSQLResponsibilitySnapshot(ctx, a.inventory.Identities["mysql"], sqlevaluation.DefaultSQLResponsibilityLimits())
	if err != nil {
		return nil, fixedError("history_global_sql_scan_failed")
	}
	mongoLimits := retirement.MongoResponsibilityLimits{PageRows: 512, MaxRows: 2_000_000, MaxBytes: 8 << 30, MaxPages: 2_000_000, MaxGraphEntries: 4_000_000, MaxGraphBytes: 1 << 30, MaxDuration: 5 * time.Minute}
	global, err := retirement.PrepareMongoResponsibilitySnapshot(ctx, d.mongo, d.mongoConfig, mongoLimits)
	if err != nil {
		return nil, fixedError("history_global_mongo_scan_failed")
	}
	sqlReport, mongoReport := current.Report(), global.Report()
	if len(sqlReport.Ledgers) != 8 || !sqlReport.ActualTransactionReadOnlyRR || sqlReport.CompletedAt.IsZero() || len(mongoReport.Collections) != 11 || !mongoReport.Complete {
		return nil, fixedError("history_global_coverage_incomplete")
	}
	if a.rewind() != nil {
		return nil, fixedError("history_asset_read_failed")
	}
	origin, err := retirement.PrepareSourceOriginSnapshotEpoch(ctx, binding, current, global, a.readers())
	if err != nil {
		return nil, fixedError("history_actual_source_origin_failed")
	}
	if a.rewind() != nil {
		return nil, fixedError("history_asset_read_failed")
	}
	catalog, err := retirement.PrepareSQLCrossStoreResponsibilityCatalog(ctx, current, sqlevaluation.DefaultSQLCrossStoreLimits())
	if err != nil {
		return nil, fixedError("history_sql_cross_store_catalog_failed")
	}
	// Re-read exact authenticated AI copies to EOF and bind their actual point
	// graph to this same host-owned RR-RO SQL epoch. Lifecycle remains here.
	if a.rewind() != nil {
		return nil, fixedError("history_asset_read_failed")
	}
	aiCopies := a.copies()
	aiReadonly, err := c.PrepareAIReadOnlyResolver(ctx, current, a.inventory.Migrations["mysql"], aiCopies[1:3])
	if err != nil {
		return nil, fixedError("history_ai_readonly_resolver_failed")
	}
	if a.rewind() != nil {
		return nil, fixedError("history_asset_read_failed")
	}
	// The point graph alone cannot see unreferenced current MQ/inbox/orphans.
	// Scan the exact complete physical 14-table layout in this same SQL8 RRRO
	// epoch, then independently re-read all four authenticated source copies.
	aiReverse, err := retirement.PrepareAIReverseSnapshot(ctx, current, a.inventory.Migrations["mysql"], retirement.DefaultAIReverseLimits())
	if err != nil {
		return nil, fixedError("history_ai_reverse_scan_failed")
	}
	if a.rewind() != nil {
		return nil, fixedError("history_asset_read_failed")
	}
	if c.BindAIReverseSourceScope(ctx, aiReverse, a.copies()) != nil {
		return nil, fixedError("history_ai_reverse_source_scope_failed")
	}
	aiReverseFacts, err := stableAIReverseObservation(aiReverse.Summary())
	if err != nil || aiReverseFacts.IdentitySHA256 != sqlReport.DatabaseIdentitySHA256 {
		return nil, fixedError("history_ai_reverse_coverage_incomplete")
	}
	if a.rewind() != nil {
		return nil, fixedError("history_asset_read_failed")
	}

	var externalQualification *retirement.AIExternalExecutionQualification
	var aiPages []*retirement.AIExternalPageQualification
	if aiCopies[1].Expected.Records+aiCopies[2].Expected.Records > 0 {
		if len(external) != 1 {
			return nil, fixedError("history_ai_external_execution_input_required")
		}
		input := external[0]
		canonicalRun, runErr := aiHostRun(a.request.RunID)
		if runErr != nil || input.RunID != canonicalRun {
			return nil, fixedError("history_ai_external_execution_input_required")
		}
		if retirement.RequireAIExternalExecQuiescence(ctx, retirement.AIExternalExecQuiescenceInput{OperationDirectory: input.OperationDirectory, SourceSHA: a.request.SourceSHA, OperationID: a.request.OperationID, RuntimeSourceSHA: input.RuntimeSourceSHA, ImageID: input.ImageID, ContainerID: input.ContainerID, SudoDocker: input.SudoDocker}) != nil {
			return nil, fixedError("history_ai_external_exec_quiescence_unproven")
		}
		externalQualification, err = c.PrepareAIExternalExecution(ctx, aiReadonly, aiReverse, input)
		if err != nil {
			return nil, fixedError("history_ai_external_execution_failed")
		}
		if a.rewind() != nil {
			return nil, fixedError("history_asset_read_failed")
		}
	}
	w := &preparedWriteEpoch{coordinator: c, index: index, catalog: catalog, global: global}
	e := &epochResult{origin: origin, sql: current, mongo: global, aiFacts: aiReadonly.Summary(), aiReverseFacts: aiReverseFacts, aiReverse: aiReverse, aiReverseCoordinator: c, reasons: map[string]uint64{}}
	if aiReverseFacts.Unknown > 0 {
		e.reasons["ai_reverse_global_unknown_responsibility"] = aiReverseFacts.Unknown
	}
	if aiReverseFacts.Blocking > 0 {
		e.reasons["ai_reverse_global_blocking_responsibility"] = aiReverseFacts.Blocking
	}
	for _, reason := range aiReverseFacts.BlockingReasons {
		e.reasons["ai_reverse_"+reason]++
	}
	// Global orphan/unknown responsibility remains visible even when all
	// four legacy sources are empty and there are no candidate-local reasons.
	if sqlReport.Unknown > 0 {
		e.reasons["sql_global_unknown_responsibility"] = sqlReport.Unknown
	}
	if sqlReport.Blocking > 0 {
		e.reasons["sql_global_blocking_responsibility"] = sqlReport.Blocking
	}
	for _, reason := range mongoReport.BlockingReasons {
		e.reasons["mongo_global_"+reason]++
	}
	for _, gap := range mongoReport.CoverageGaps {
		e.reasons["mongo_global_coverage_gap_"+gap]++
	}
	for {
		page, err := c.NextPage(ctx)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fixedError("history_coordinator_page_failed")
		}
		events, err := page.Events()
		if err != nil {
			return nil, fixedError("history_coordinator_page_failed")
		}
		if len(events) == 0 {
			if externalQualification == nil {
				return nil, fixedError("history_ai_external_execution_input_required")
			}
			prepared, prepareErr := c.PrepareAICommandPersistencePage(ctx, page, externalQualification)
			if prepareErr != nil {
				return nil, fixedError("history_ai_original_persistence_prepare_failed")
			}
			aiPages = append(aiPages, prepared)
			// Local readonly facts do not prove the external qs-ai execution or
			// whole reverse MQ graph, and cannot authorize evidence CAS or DROP.
			if c.QualifyAIReadOnlyPage(ctx, page, aiReadonly) != nil {
				return nil, fixedError("history_ai_readonly_page_failed")
			}
			e.aiPages++
			// Consumption retains every original candidate/blocker. The separately
			// derived opaque persistence batch neither clears these nor sends commands.
			continue
		}
		selectors, err := retirement.MongoHistoricalSQLBatchSelectors(events)
		if err != nil {
			return nil, fixedError("history_original_business_selection_failed")
		}
		sqlBatch, err := retirement.PrepareSQLBusinessOwnerBatch(ctx, current, selectors, sqlevaluation.DefaultSQLHistoricalOwnerBatchLimits())
		if err != nil {
			return nil, fixedError("history_original_sql_business_failed")
		}
		joint, err := c.PrepareWholeSourceJointPage(ctx, page, index, catalog, sqlBatch, global)
		if err != nil {
			if errors.Is(err, retirement.ErrWholeSourceJointBounds) {
				return nil, fixedError("history_related_source_budget_exceeded")
			}
			return nil, fixedError("history_joint_original_qualification_failed")
		}
		if c.QualifyWholeSourceJointPage(ctx, page, joint) != nil {
			return nil, fixedError("history_joint_page_consumption_failed")
		}
		anchor, sealErr := c.SealWholeSourceJointReplayAnchor(ctx, joint)
		if sealErr != nil {
			return nil, fixedError("history_write_replay_seal_failed")
		}
		w.anchors = append(w.anchors, anchor)
		e.jointPages++
	}
	e.coordinator = c.Receipt()
	e.index = index.Summary()
	if !e.coordinator.SourceCoverageComplete || !e.index.Complete || e.coordinator.DropReady || e.coordinator.BusinessClosureVerified {
		return nil, fixedError("history_four_source_coverage_incomplete")
	}
	for offset := 0; uint64(offset) < e.coordinator.CandidateCount; offset += 128 {
		rows, err := c.CandidateRange(offset, 128)
		if err != nil {
			return nil, fixedError("history_candidate_summary_failed")
		}
		for _, row := range rows {
			for _, reason := range row.BlockingReasons {
				e.reasons[reason]++
			}
		}
	}
	w.aiBatch, err = c.SealAICommandPersistencePages(ctx, aiPages)
	if err != nil {
		return nil, fixedError("history_ai_original_persistence_prepare_failed")
	}
	e.sqlFacts = stableSQLFacts{sqlReport.DatabaseIdentitySHA256, sqlReport.BusinessAnchorsSHA256, sqlReport.SchemaCoverage, sqlReport.Ledgers, sqlReport.Observed, sqlReport.RetirementRelated, sqlReport.OutsideRetirement, sqlReport.Unknown, sqlReport.Blocking}
	e.mongoFacts = stableMongoFacts{mongoReport.IdentitySHA256, mongoReport.MetadataSHA256, mongoReport.SnapshotSHA256, mongoReport.MigrationVersion, mongoReport.Collections, mongoReport.ClassCounts, mongoReport.BlockingReasons, mongoReport.CoverageGaps, mongoReport.Rows, mongoReport.Bytes, mongoReport.Pages, mongoReport.ClassifiedRows}
	if current.ValidateBorrowedSnapshot(ctx) != nil || global.ValidateBorrowedSnapshot(ctx) != nil || aiReverse.ValidateBorrowedSnapshot(ctx) != nil || a.verifyFullFiles(ctx) != nil {
		return nil, fixedError("history_epoch_changed_before_close")
	}
	w.facts = e
	return w, nil
}

// The explicit host may run prewindow evidence CAS under its existing baseline,
// admission and original-command gates. This result grants no DROP authority;
// deletion still requires independent whole-writer fencing and a real window.
func executeHistoricalEvidenceWrite(ctx context.Context, a *approvedInputs, d *historyDatabase, outputDir string, external ...retirement.AIExternalExecutionInput) (report preparedWriteDiagnostic, result error) {
	report = preparedWriteDiagnostic{Protocol: "limited-historical-evidence-host/v1", CommitState: "not_attempted", MongoCommitRequirement: "undetermined", Required: []string{"independent_production_source_approval", "whole_writer_and_historical_rerun_fence", "production_process_and_transaction_budget", "actual_external_qs_ai_facts_for_unmapped_original_commands", "complete_original_command_evidence_persistence_and_readback", "final_expected_global_and_absent_range_fence", "prewindow_actual_isolated_restore", "maintenance_acceptance_and_purge"}}
	if ctx == nil || ctx.Err() != nil || a == nil || d == nil || len(external) > 1 {
		return report, fixedError("history_write_input_rejected")
	}
	report.SourceSHA, report.OperationID, report.RunID = a.request.SourceSHA, a.request.OperationID, a.request.RunID
	journal, e := newHistoryWriteJournal(outputDir, a)
	if e != nil {
		return report, e
	}
	defer func() {
		// Later defers have already closed both original spool writers. A close
		// failure or unknown DB result cannot publish a material handoff digest.
		if result == nil {
			var err error
			report.MaterialManifestSHA256, err = journal.snapshotMaterials(ctx)
			if err != nil {
				result = err
			}
		}
		if journal.dir.Close() != nil && result == nil {
			result = fixedError("history_write_private_close_failed")
		}
		if result != nil {
			report.MaterialManifestSHA256 = ""
		}
	}()
	mongoFile, e := journal.create("prepared-mongo-private.bin")
	if e != nil {
		return report, e
	}
	defer func() {
		if mongoFile.Close() != nil && result == nil {
			result = fixedError("history_write_private_close_failed")
		}
	}()
	sqlFile, e := journal.create("prepared-sql-private.bin")
	if e != nil {
		return report, e
	}
	defer func() {
		if sqlFile.Close() != nil && result == nil {
			result = fixedError("history_write_private_close_failed")
		}
	}()
	spool, e := retirement.NewHistoricalCASSpool(ctx, mongoFile, sqlFile, 16<<30, 128<<20)
	if e != nil {
		return report, fixedError("history_write_spool_failed")
	}
	var first *epochResult
	var aiBatch *retirement.AICommandPersistenceBatch
	var originAnchor *retirement.FreshRecheckAnchor
	eventExpected := a.copies()[0].Expected.Records + a.copies()[3].Expected.Records
	report.MongoCommitRequirement = "not_required"
	if eventExpected > 0 {
		report.MongoCommitRequirement = "required"
	}
	if e = d.epoch(ctx, func(scope context.Context) error {
		var err error
		first, err = buildEpoch(scope, a, d)
		if err != nil {
			return err
		}
		return first.compactOrigin(scope)
	}); e != nil {
		return report, e
	}
	// The second actual fresh RO constructs every plan while still alive, but
	// immediately writes raw bodies/baselines to private bounded disk capsules.
	// Its related graphs are page-bounded, not retained as 1,242,978 future plans.
	if e = d.epoch(ctx, func(scope context.Context) error {
		second, err := buildPreparedWriteEpoch(scope, a, d, external...)
		if err != nil {
			return err
		}
		if compareEpochs(first, second.facts) != nil {
			return fixedError("history_independent_epoch_facts_changed")
		}
		ai, err := first.recheckAIReverse(scope, second.facts, a)
		if err != nil {
			return err
		}
		if a.rewind() != nil {
			return fixedError("history_asset_read_failed")
		}
		origin, err := first.reverseAnchor.RecheckOrigin(scope, ai, second.facts.sql, second.global, second.coordinator, a.readers())
		if err != nil {
			return fixedError("history_actual_origin_independent_epoch_failed")
		}
		for _, anchor := range second.anchors {
			sequence, offset, err := anchor.Selectors()
			if err != nil {
				return fixedError("history_write_replay_failed")
			}
			joint, err := second.coordinator.ReplayWholeSourceJointPage(scope, anchor, sequence, offset, second.index, second.catalog, second.global)
			if err != nil {
				return fixedError("history_write_replay_failed")
			}
			// Actual original/fresh source, global and business capabilities, not flags.
			_, _, sealed, err := second.coordinator.PrepareQualifiedHistoricalCAS(scope, joint, origin, ai)
			if err != nil {
				return fixedError("history_write_qualification_failed")
			}
			ticket, err := spool.Append(scope, joint, sealed)
			if err != nil {
				return fixedError("history_write_spool_failed")
			}
			position, refs := ticket.Diagnostic()
			if journal.record(scope, "prepared", position, refs, nil) != nil {
				return fixedError("history_write_journal_unknown")
			}
			report.PreparedPages++
			report.EventReferences += refs
		}
		if eventExpected > 0 {
			if spool.Seal(scope, second.coordinator) != nil {
				return fixedError("history_write_spool_failed")
			}
		}
		aiBatch = second.aiBatch
		if eventExpected == 0 && aiBatch == nil {
			return fixedError("history_write_no_original_statements")
		}

		// Keep only the actual second origin epoch seal. Its original expiry and
		// exact four source receipts survive; no old transaction or row graph does.
		originAnchor, err = second.facts.origin.FreezeFreshRecheckAnchor(scope)
		if err != nil {
			return fixedError("history_write_origin_anchor_failed")
		}

		// Do not keep old whole SQL8/Mongo11/AI14/source/index or page replay anchors
		// past this callback. The spool retains small actual epoch/file capabilities.
		second = nil
		return a.verifyFullFiles(scope)
	}); e != nil {
		return report, e
	}
	first = nil
	var tickets []*retirement.HistoricalCASSpoolTicket
	if eventExpected > 0 {
		tickets, e = spool.Tickets()
		if e != nil {
			return report, fixedError("history_write_spool_failed")
		}
	}
	var bounded context.Context
	var cancel context.CancelFunc
	if eventExpected > 0 {
		bounded, cancel, e = spool.InheritedContext(ctx)
	} else {
		bounded, cancel, e = aiBatch.InheritedContext(ctx)
	}
	if e != nil {
		return report, fixedError("history_write_spool_failed")
	}
	defer cancel()
	if a.verifyFullFiles(bounded) != nil {
		return report, fixedError("history_asset_hash_changed")
	}
	applied, e := d.writePreparedEpoch(bounded, spool, tickets, journal, &report, aiBatch)
	if e != nil {
		return report, e
	}
	// Both actual host commit calls returned success. They are NOT distributed
	// atomic commit. No retry/reconstruction occurs on a failed/unknown response.
	if e = d.epoch(bounded, func(scope context.Context) error {
		current, err := retirement.PrepareSQLResponsibilitySnapshot(scope, a.inventory.Identities["mysql"], sqlevaluation.DefaultSQLResponsibilityLimits())
		if err != nil {
			return fixedError("history_global_sql_scan_failed")
		}
		limits := retirement.MongoResponsibilityLimits{PageRows: 512, MaxRows: 2_000_000, MaxBytes: 8 << 30, MaxPages: 2_000_000, MaxGraphEntries: 4_000_000, MaxGraphBytes: 1 << 30, MaxDuration: 5 * time.Minute}
		global, err := retirement.PrepareMongoResponsibilitySnapshot(scope, d.mongo, d.mongoConfig, limits)
		if err != nil {
			return fixedError("history_global_mongo_scan_failed")
		}
		// No JOIN/organization filters or mutable summary switches replace these
		// actual scans. New current responsibility still blocks final observation.
		sr, mr := current.Report(), global.Report()
		if sr.Unknown != 0 || sr.Blocking != 0 || len(mr.BlockingReasons) != 0 || !mr.Complete {
			return fixedError("history_write_final_global_responsibility_changed")
		}
		// SQL8/Mongo11 do not scan the old event ledgers. Re-read the actual
		// four old sources under their frozen bounds, using the genuine second
		// epoch seal and new SQL/Mongo transactions, before any completion record.
		if verifyWrittenSourceOrigin(scope, a, originAnchor, current, global) != nil {
			return fixedError("history_write_actual_origin_readback_failed")
		}
		if journal.record(scope, "actual_origin_readback_matched", -1, eventExpected, nil) != nil {
			return fixedError("history_write_journal_unknown")
		}
		if a.verifyFullFiles(scope) != nil || a.rewind() != nil {
			return fixedError("history_asset_hash_changed")
		}
		c, err := retirement.PrepareHistoricalCoordinator(scope, retirement.HistoricalCoordinatorBinding{SourceSHA: a.request.SourceSHA, OperationID: a.request.OperationID}, a.copies(), retirement.DefaultHistoricalCoordinatorLimits())
		if err != nil {
			return fixedError("history_source_authentication_failed")
		}
		if a.rewind() != nil {
			return fixedError("history_asset_read_failed")
		}
		jointLimits := retirement.DefaultWholeSourceJointLimits()
		jointLimits.MaxRelatedSources = 512
		index, err := c.PrepareWholeSourceJointIndex(scope, a.jointCopies(), jointLimits)
		if err != nil {
			return fixedError("history_complete_source_index_failed")
		}
		for _, ticket := range tickets {
			observation, err := spool.VerifyTicket(scope, applied, ticket, current, global, index)
			if err != nil {
				return fixedError("history_write_independent_readback_failed")
			}
			position, refs := ticket.Diagnostic()
			if journal.record(scope, "fresh_page_verified", position, refs, &observation) != nil {
				return fixedError("history_write_journal_unknown")
			}
			report.ReadBackPages++
		}
		if eventExpected > 0 {
			if spool.FinishReadback(scope, applied) != nil {
				return fixedError("history_write_independent_readback_failed")
			}
		}
		if aiBatch != nil {
			reverse, scanErr := retirement.PrepareAIReverseSnapshot(scope, current, a.inventory.Migrations["mysql"], retirement.DefaultAIReverseLimits())
			if scanErr != nil {
				return fixedError("history_ai_reverse_scan_failed")
			}
			if a.rewind() != nil || c.BindAIReverseSourceScope(scope, reverse, a.copies()) != nil {
				return fixedError("history_ai_reverse_source_scope_failed")
			}
			observation, readErr := aiBatch.VerifyReadback(scope, reverse)
			if readErr != nil || !observation.IndependentReadbackMatched || observation.OriginalCommands != report.AIOriginalCommands || observation.OriginalSourceReferences != report.AISourceReferences {
				return fixedError("history_ai_original_independent_readback_failed")
			}
			if journal.record(scope, "ai_independent_readback_finished", -1, report.AISourceReferences, nil) != nil {
				return fixedError("history_write_journal_unknown")
			}
			report.AICommandPersistenceComplete = true
		}

		if current.ValidateBorrowedSnapshot(scope) != nil || global.ValidateBorrowedSnapshot(scope) != nil || a.verifyFullFiles(scope) != nil {
			return fixedError("history_epoch_changed_before_close")
		}
		return nil
	}); e != nil {
		return report, e
	}
	if report.ReadBackPages != report.PreparedPages {
		return report, fixedError("history_write_independent_readback_failed")
	}
	if e = journal.record(bounded, "limited_event_readback_finished", -1, report.EventReferences, nil); e != nil {
		return report, e
	}
	report.LimitedEventPersistenceObserved = eventExpected > 0
	return report, nil
}

func (d *historyDatabase) writePreparedEpoch(ctx context.Context, spool *retirement.HistoricalCASSpool, tickets []*retirement.HistoricalCASSpoolTicket, journal *historyWriteJournal, report *preparedWriteDiagnostic, aiBatches ...*retirement.AICommandPersistenceBatch) (applied *retirement.HistoricalCASSpoolApplied, result error) {
	var aiBatch *retirement.AICommandPersistenceBatch
	if len(aiBatches) > 1 {
		return nil, fixedError("history_write_host_epoch_rejected")
	}
	if len(aiBatches) == 1 {
		aiBatch = aiBatches[0]
	}
	if d == nil || spool == nil || len(tickets) == 0 && aiBatch == nil || report == nil || ctx == nil || ctx.Err() != nil || d.validateNamespaceAnchor(ctx) != nil {
		return nil, fixedError("history_write_host_epoch_rejected")
	}
	if report.MongoCommitRequirement != "required" && report.MongoCommitRequirement != "not_required" ||
		(report.MongoCommitRequirement == "required") != (len(tickets) > 0) {
		return nil, fixedError("history_write_host_epoch_rejected")
	}
	session, e := d.mongoClient.StartSession()
	if e != nil {
		return nil, fixedError("history_write_host_epoch_rejected")
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		session.EndSession(cleanup)
		cancel()
	}()
	if session.StartTransaction(options.Transaction().SetReadConcern(readconcern.Snapshot()).SetReadPreference(readpref.Primary())) != nil {
		return nil, fixedError("history_write_host_epoch_rejected")
	}
	tx := d.sql.WithContext(ctx).Begin(&sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: false})
	if tx.Error != nil {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_ = session.AbortTransaction(cleanup)
		cancel()
		return nil, fixedError("history_write_host_epoch_rejected")
	}
	sqlCommitAttempt, mongoCommitAttempt := false, false
	// Abort/rollback applies only before a commit ATTEMPT. After an unknown
	// response the durable journal is authoritative about this host's uncertainty;
	// we do not automatically repeat Commit, Apply, stage new IDs or proof times.
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if !sqlCommitAttempt {
			if err := tx.Rollback().Error; err != nil && !errors.Is(err, sql.ErrTxDone) {
				_ = journal.record(cleanup, "sql_rollback_failed", -1, 0, nil)
				if result == nil {
					result = fixedError("history_write_cleanup_unknown")
				}
			}
		}
		if !mongoCommitAttempt {
			if session.AbortTransaction(cleanup) != nil {
				_ = journal.record(cleanup, "mongo_abort_failed", -1, 0, nil)
				if result == nil {
					result = fixedError("history_write_cleanup_unknown")
				}
			}
		}
	}()
	var one int
	if tx.Raw("SELECT 1").Scan(&one).Error != nil || one != 1 {
		return nil, fixedError("history_write_host_epoch_rejected")
	}
	paired := mongo.NewSessionContext(hostmysql.WithTx(ctx, tx), session)
	if journal.record(paired, "write_epoch_started", -1, 0, nil) != nil {
		return nil, fixedError("history_write_journal_unknown")
	}
	if aiBatch != nil {
		written, writeErr := aiBatch.Record(paired, tx)
		if writeErr != nil || !written.InTransactionWritten {
			return nil, fixedError("history_ai_original_persistence_statement_failed")
		}
		report.AIOriginalCommands, report.AISourceReferences = written.OriginalCommands, written.OriginalSourceReferences
		if journal.record(paired, "ai_statements_applied", -1, written.OriginalSourceReferences, nil) != nil {
			return nil, fixedError("history_write_journal_unknown")
		}
	}
	for _, ticket := range tickets {
		if spool.ApplyTicket(paired, ticket) != nil {
			return nil, fixedError("history_write_statement_failed")
		}
		position, refs := ticket.Diagnostic()
		if journal.record(paired, "page_statement_applied", position, refs, nil) != nil {
			return nil, fixedError("history_write_journal_unknown")
		}
	}
	if len(tickets) > 0 {
		applied, e = spool.FinishApply(paired)
		if e != nil {
			return nil, fixedError("history_write_statement_failed")
		}
	}

	if journal.record(paired, "all_statements_applied", -1, report.EventReferences, nil) != nil {
		return nil, fixedError("history_write_journal_unknown")
	}
	// Event counts do not prove a Mongo command ran: a SQL-only owner page
	// may have an empty Mongo selection. Starting is only a local driver state.
	// Reject it before either commit so the host's original cleanup rolls back.
	if err := requireMongoCommitTransaction(session, report.MongoCommitRequirement); err != nil {
		return nil, err
	}
	if journal.record(paired, "sql_commit_intent", -1, 0, nil) != nil {
		return nil, fixedError("history_write_journal_unknown")
	}
	sqlCommitAttempt = true
	if tx.Commit().Error != nil {
		report.CommitState = "sql_unknown_mongo_not_attempted"
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_ = journal.record(cleanup, "sql_commit_unknown", -1, 0, nil)
		cancel()
		return nil, fixedError("history_write_commit_unknown")
	}
	report.ActualSQLCommitResponse = true
	if journal.record(ctx, "sql_commit_success", -1, 0, nil) != nil {
		report.CommitState = "sql_response_success_journal_unknown"
		return nil, fixedError("history_write_journal_unknown")
	}
	// No event pages means no Mongo transaction command was executed. The
	// driver's client-only nil for an empty transaction is not a server response.
	if report.MongoCommitRequirement == "required" {
		if journal.record(ctx, "mongo_commit_intent", -1, 0, nil) != nil {
			report.CommitState = "sql_committed_mongo_not_attempted"
			return nil, fixedError("history_write_journal_unknown")
		}
		mongoCommitAttempt = true
	}
	if err := commitPreparedMongo(ctx, session, journal, report); err != nil {
		return nil, err
	}
	if d.validateNamespaceAnchor(ctx) != nil {
		return nil, fixedError("history_database_binding_rejected")
	}
	return applied, nil
}

// This callback only accepts the opaque second actual origin seal. The existing
// producer re-reads native SQL/BSON sources and compares every frozen receipt.
func verifyWrittenSourceOrigin(ctx context.Context, a *approvedInputs, anchor *retirement.FreshRecheckAnchor, current *retirement.SQLResponsibilitySnapshot, global *retirement.MongoResponsibilitySnapshot) error {
	if a == nil || anchor == nil || current == nil || global == nil || a.verifyFullFiles(ctx) != nil {
		return fixedError("history_write_actual_origin_readback_failed")
	}
	proof, err := anchor.RecheckSnapshots(ctx, current, global, a.readers())
	if err != nil || proof == nil {
		return fixedError("history_write_actual_origin_readback_failed")
	}
	facts := proof.Report()
	if !facts.ActualOriginMatched || !facts.SourceFilesMatched || !facts.IndependentEpochRechecked || facts.CASAuthorized || facts.DropReady {
		return fixedError("history_write_actual_origin_readback_failed")
	}
	return nil
}

func commitPreparedMongo(ctx context.Context, session mongo.Session, journal *historyWriteJournal, report *preparedWriteDiagnostic) error {
	if report == nil || !report.ActualSQLCommitResponse || session == nil || journal == nil {
		return fixedError("history_write_host_epoch_rejected")
	}
	if report.MongoCommitRequirement == "not_required" {
		if report.EventReferences != 0 || report.PreparedPages != 0 || report.ActualMongoCommitResponse {
			return fixedError("history_write_host_epoch_rejected")
		}
		report.CommitState = "sql_committed_mongo_not_required"
		return journal.record(ctx, "mongo_commit_not_required", -1, 0, nil)
	}
	if report.MongoCommitRequirement != "required" {
		return fixedError("history_write_host_epoch_rejected")
	}
	if session.CommitTransaction(ctx) != nil {
		report.CommitState = "sql_committed_mongo_unknown"
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_ = journal.record(cleanup, "mongo_commit_unknown", -1, 0, nil)
		cancel()
		return fixedError("history_write_commit_unknown")
	}
	report.ActualMongoCommitResponse = true
	report.CommitState = "both_responses_success_non_atomic"
	return journal.record(ctx, "mongo_commit_success", -1, 0, nil)
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
