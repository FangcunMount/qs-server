package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
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

// This is NOT connected to main/CLI/Action/CAPABILITIES. It is an actual host
// implementation candidate for limited event/old-command evidence persistence,
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
	Protocol, SourceSHA, OperationID, RunID, CommitState                                                                           string
	AIOriginalCommands, AISourceReferences                                                                                         uint64
	PreparedPages, ReadBackPages, EventReferences                                                                                  uint64
	ActualSQLCommitResponse, ActualMongoCommitResponse                                                                             bool
	LimitedEventPersistenceObserved                                                                                                bool
	AICommandPersistenceComplete, FullExternalAIClosureVerified, WriterFenceProven, CASComplete, DropReady, MutationBackendEnabled bool
	Required                                                                                                                       []string
}
type historyWriteJournal struct {
	path     string
	dir      *os.File
	dev, ino uint64
	sequence uint64
	binding  retirement.HistoricalCoordinatorBinding
	run      string
	unknown  bool
}
type historyWriteJournalRecord struct {
	Version        int                                           `json:"version"`
	SourceSHA      string                                        `json:"source_sha"`
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
	return &historyWriteJournal{path: dir, dir: f, dev: uint64(st.Dev), ino: uint64(st.Ino), binding: retirement.HistoricalCoordinatorBinding{SourceSHA: a.request.SourceSHA, OperationID: a.request.OperationID}, run: a.request.RunID}, nil
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
	return os.NewFile(uintptr(fd), name), nil
}
func (j *historyWriteJournal) record(ctx context.Context, stage string, position int, refs uint64, observation *retirement.HistoricalCASSpoolPageObservation) error {
	if ctx == nil || ctx.Err() != nil || !j.valid() {
		return fixedError("history_write_journal_unknown")
	}
	switch stage {
	case "prepared", "write_epoch_started", "page_statement_applied", "all_statements_applied", "sql_commit_intent", "sql_commit_success", "sql_commit_unknown", "mongo_commit_intent", "mongo_commit_success", "mongo_commit_unknown", "mongo_abort_failed", "sql_rollback_failed", "fresh_page_verified", "limited_event_readback_finished", "ai_statements_applied", "ai_independent_readback_finished":
	default:
		return fixedError("history_write_journal_unknown")
	}
	rec := historyWriteJournalRecord{Version: 1, SourceSHA: j.binding.SourceSHA, OperationID: j.binding.OperationID, RunID: j.run, Sequence: j.sequence + 1, Stage: stage, PagePosition: position, References: refs, Observation: observation}
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

// This real caller has no public invocation. The future production host must
// additionally bind independent approval/writer fence; neither a
// request JSON nor this diagnostic return can confer those missing authorities.
func executeHistoricalEvidenceWrite(ctx context.Context, a *approvedInputs, d *historyDatabase, outputDir string, external ...retirement.AIExternalExecutionInput) (report preparedWriteDiagnostic, result error) {
	report = preparedWriteDiagnostic{Protocol: "limited-historical-evidence-host/v1", CommitState: "not_attempted", Required: []string{"independent_production_source_approval", "whole_writer_and_historical_rerun_fence", "production_process_and_transaction_budget", "actual_external_qs_ai_facts_for_unmapped_original_commands", "complete_original_command_evidence_persistence_and_readback", "final_expected_global_and_absent_range_fence", "prewindow_actual_isolated_restore", "maintenance_acceptance_and_purge"}}
	if ctx == nil || ctx.Err() != nil || a == nil || d == nil || len(external) > 1 {
		return report, fixedError("history_write_input_rejected")
	}
	report.SourceSHA, report.OperationID, report.RunID = a.request.SourceSHA, a.request.OperationID, a.request.RunID
	journal, e := newHistoryWriteJournal(outputDir, a)
	if e != nil {
		return report, e
	}
	defer func() {
		if journal.dir.Close() != nil && result == nil {
			result = fixedError("history_write_private_close_failed")
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
	eventExpected := a.copies()[0].Expected.Records + a.copies()[3].Expected.Records
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

		// Do not keep old whole SQL8/Mongo11/AI14/source/index or body-free anchors
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
	if journal.record(ctx, "mongo_commit_intent", -1, 0, nil) != nil {
		report.CommitState = "sql_committed_mongo_not_attempted"
		return nil, fixedError("history_write_journal_unknown")
	}
	mongoCommitAttempt = true
	if session.CommitTransaction(ctx) != nil {
		report.CommitState = "sql_committed_mongo_unknown"
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_ = journal.record(cleanup, "mongo_commit_unknown", -1, 0, nil)
		cancel()
		return nil, fixedError("history_write_commit_unknown")
	}
	report.ActualMongoCommitResponse = true
	report.CommitState = "both_responses_success_non_atomic"
	if journal.record(ctx, "mongo_commit_success", -1, 0, nil) != nil {
		return nil, fixedError("history_write_journal_unknown")
	}
	if d.validateNamespaceAnchor(ctx) != nil {
		return nil, fixedError("history_database_binding_rejected")
	}
	return applied, nil
}
