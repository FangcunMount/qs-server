package main

import (
	"context"
	"errors"
	"io"
	"reflect"
	"sort"
	"time"

	sqlevaluation "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	retirement "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirement"
)

// Public output contains derived hashes/counts/categories only. No input DTO is
// accepted as a successful qualification or production write authorization.
type readiness struct {
	Protocol                                string                          `json:"protocol"`
	SourceSHA                               string                          `json:"source_sha"`
	OperationID                             string                          `json:"operation_id"`
	RunID                                   string                          `json:"run_id"`
	RequestSHA256                           string                          `json:"request_sha256"`
	InventoryRequestSHA256                  string                          `json:"inventory_request_sha256"`
	InventoryReportSHA256                   string                          `json:"inventory_report_sha256"`
	CompletedReadOnlyPipeline               bool                            `json:"completed_readonly_pipeline"`
	IndependentEpochs                       int                             `json:"independent_epochs"`
	SourceFilesAndActualOriginsMatched      bool                            `json:"source_files_and_actual_origins_matched"`
	WholeFourSourceCoverageComplete         bool                            `json:"whole_four_source_coverage_complete"`
	BusinessAndResponsibilityFactsUnchanged bool                            `json:"business_and_responsibility_facts_unchanged"`
	Sources                                 [4]retirement.SourceCopyReceipt `json:"sources"`
	FullSourceFileSHA256                    [4]string                       `json:"full_source_file_sha256"`
	WholeSourceIndexSHA256                  string                          `json:"whole_source_index_sha256"`
	CandidateSHA256                         string                          `json:"candidate_sha256"`
	SQLCurrentFactsSHA256                   string                          `json:"sql_current_facts_sha256"`
	MongoCurrentFactsSHA256                 string                          `json:"mongo_current_facts_sha256"`
	LocalCandidates                         uint64                          `json:"local_candidates"`
	LocallyQualified                        uint64                          `json:"locally_qualified"`
	BlockedLocal                            uint64                          `json:"blocked_local"`
	JointEventPages                         uint64                          `json:"joint_event_pages"`
	AIBlockedPages                          uint64                          `json:"ai_blocked_pages"`
	SQLLedgerCount                          int                             `json:"sql_ledger_count"`
	MongoCollectionCount                    int                             `json:"mongo_collection_count"`
	SQLGlobal                               sqlGlobalSummary                `json:"sql_global"`
	MongoGlobal                             mongoGlobalSummary              `json:"mongo_global"`
	AIReverseGlobal                         aiReverseGlobalSummary          `json:"ai_reverse_global"`
	BlockingReasons                         map[string]uint64               `json:"blocking_reasons"`
	RequiredAdapters                        []string                        `json:"required_adapters"`
	IndependentProductionApprovalVerified   bool                            `json:"independent_production_approval_verified"`
	OrderedMongoSourceMetadataApproved      bool                            `json:"ordered_mongo_source_metadata_approved"`
	HostProcessBudgetProven                 bool                            `json:"host_process_budget_proven"`
	FullExternalAIClosureVerified           bool                            `json:"full_external_ai_closure_verified"`
	DistributedAtomicSnapshot               bool                            `json:"distributed_atomic_snapshot"`
	WriterFenceProven                       bool                            `json:"writer_fence_proven"`
	CASComplete                             bool                            `json:"cas_complete"`
	PostCASReadbackComplete                 bool                            `json:"post_cas_readback_complete"`
	BackupRestoreQualified                  bool                            `json:"backup_restore_qualified"`
	MutationBackendEnabled                  bool                            `json:"mutation_backend_enabled"`
	DropReady                               bool                            `json:"drop_ready"`
	ErrorCategory                           string                          `json:"error_category"`
	ElapsedMilliseconds                     int64                           `json:"elapsed_milliseconds"`
}
type sqlGlobalSummary struct {
	Observed          uint64 `json:"observed"`
	RetirementRelated uint64 `json:"retirement_related"`
	OutsideRetirement uint64 `json:"outside_retirement"`
	Unknown           uint64 `json:"unknown"`
	Blocking          uint64 `json:"blocking"`
	SchemaCoverage    string `json:"schema_coverage"`
}
type mongoGlobalSummary struct {
	Rows            uint64            `json:"rows"`
	ClassifiedRows  uint64            `json:"classified_rows"`
	ClassCounts     map[string]uint64 `json:"class_counts"`
	BlockingReasons []string          `json:"blocking_reasons"`
	CoverageGaps    []string          `json:"coverage_gaps"`
}

// These public fields are fixed hashes, counts and local coverage only. They
// never serialize request/command/session/Run IDs or encrypted/plain bodies.
type aiReverseGlobalSummary struct {
	LedgerCount               int    `json:"ledger_count"`
	Rows                      uint64 `json:"rows"`
	Related                   uint64 `json:"retirement_related"`
	Outside                   uint64 `json:"outside_retirement"`
	Unknown                   uint64 `json:"unknown"`
	Blocking                  uint64 `json:"blocking"`
	OutsideActive             uint64 `json:"outside_active"`
	DataSHA256                string `json:"data_sha256"`
	SourceScopeSHA256         string `json:"source_scope_sha256"`
	WholeLedgerEOF            bool   `json:"whole_ledger_eof"`
	IndependentEpochRechecked bool   `json:"independent_epoch_rechecked"`
}
type stableAIReverseFacts struct {
	IdentitySHA256, DataSHA256, BusinessAnchorsSHA256, SourceScopeSHA256 string
	MigrationVersion                                                     uint64
	Ledgers                                                              []retirement.AIReverseLedgerSummary
	Sources                                                              [4]retirement.SourceCopyReceipt
	Rows, Bytes, Related, Outside, Unknown, Blocking, OutsideActive      uint64
	BlockingReasons                                                      []string
}
type stableSQLFacts struct {
	IdentitySHA256                                string
	BusinessAnchorsSHA256                         string
	SchemaCoverage                                string
	Ledgers                                       []sqlevaluation.SQLResponsibilityLedgerReport
	Observed, Related, Outside, Unknown, Blocking uint64
}
type stableMongoFacts struct {
	IdentitySHA256, MetadataSHA256, SnapshotSHA256 string
	MigrationVersion                               int64
	Collections                                    []retirement.MongoResponsibilityCollectionReport
	ClassCounts                                    map[string]uint64
	BlockingReasons, CoverageGaps                  []string
	Rows, Bytes, Pages, ClassifiedRows             uint64
}
type epochResult struct {
	coordinator          retirement.HistoricalCoordinatorReceipt
	index                retirement.WholeSourceJointIndexSummary
	aiFacts              retirement.AIReadOnlyBindingSummary
	aiReverseFacts       stableAIReverseFacts
	aiReverse            *retirement.AIReverseSnapshot
	aiReverseCoordinator *retirement.HistoricalCoordinator
	sqlFacts             stableSQLFacts
	mongoFacts           stableMongoFacts
	origin               *retirement.SourceOriginEpoch
	anchor               *retirement.FreshRecheckAnchor // always nil after joint compaction; no old auth retention
	reverseAnchor        *retirement.AIReverseRecheckAnchor
	sql                  *retirement.SQLResponsibilitySnapshot
	mongo                *retirement.MongoResponsibilitySnapshot
	jointPages, aiPages  uint64
	reasons              map[string]uint64
}

func emptyReadiness(a *approvedInputs) readiness {
	r := readiness{Protocol: "qs-compatibility-history-readonly/v1", MongoGlobal: mongoGlobalSummary{ClassCounts: map[string]uint64{}, BlockingReasons: []string{}, CoverageGaps: []string{}}, ErrorCategory: "history_incomplete", BlockingReasons: map[string]uint64{}, RequiredAdapters: []string{"independent_authenticated_production_request_approval", "ordered_mongo_original_source_metadata_independent_approval", "production_process_budget", "actual_mongo_transaction_lifetime_and_whole_epoch_scale", "whole_process_peak_rss_and_related_owner_width", "qs_ai_complete_original_execution_and_message_closure", "ai_stored_wire_original_jose_authentication", "production_platform_account_and_writer_fence", "historical_evidence_cas", "post_cas_independent_business_readback", "actual_production_backup_isolated_restore_and_budget", "maintenance_acceptance_and_private_asset_purge"}}
	if a == nil {
		return r
	}
	r.SourceSHA = a.request.SourceSHA
	r.OperationID = a.request.OperationID
	r.RunID = a.request.RunID
	r.RequestSHA256 = a.requestSHA
	r.InventoryRequestSHA256 = a.request.InventoryRequest.SHA256
	r.InventoryReportSHA256 = a.request.InventoryReport.SHA256
	for i, asset := range a.request.Assets {
		r.FullSourceFileSHA256[i] = asset.FullFileSHA256
	}
	return r
}
func buildEpoch(ctx context.Context, a *approvedInputs, d *historyDatabase) (*epochResult, error) {
	if a == nil || d == nil {
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
			// Local readonly facts do not prove the external qs-ai execution or
			// whole reverse MQ graph, and cannot authorize evidence CAS or DROP.
			if c.QualifyAIReadOnlyPage(ctx, page, aiReadonly) != nil {
				return nil, fixedError("history_ai_readonly_page_failed")
			}
			e.aiPages++
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
	e.sqlFacts = stableSQLFacts{sqlReport.DatabaseIdentitySHA256, sqlReport.BusinessAnchorsSHA256, sqlReport.SchemaCoverage, sqlReport.Ledgers, sqlReport.Observed, sqlReport.RetirementRelated, sqlReport.OutsideRetirement, sqlReport.Unknown, sqlReport.Blocking}
	e.mongoFacts = stableMongoFacts{mongoReport.IdentitySHA256, mongoReport.MetadataSHA256, mongoReport.SnapshotSHA256, mongoReport.MigrationVersion, mongoReport.Collections, mongoReport.ClassCounts, mongoReport.BlockingReasons, mongoReport.CoverageGaps, mongoReport.Rows, mongoReport.Bytes, mongoReport.Pages, mongoReport.ClassifiedRows}
	if current.ValidateBorrowedSnapshot(ctx) != nil || global.ValidateBorrowedSnapshot(ctx) != nil || aiReverse.ValidateBorrowedSnapshot(ctx) != nil || a.verifyFullFiles(ctx) != nil {
		return nil, fixedError("history_epoch_changed_before_close")
	}
	return e, nil
}

// Release every original business/source graph only after an actual joint
// opaque AI/origin seal. The retained borrowed Tx checks host-ended lifecycle;
// fixed metadata and digests are not a production approval or memory proof.
func (e *epochResult) compactOrigin(ctx context.Context) error {
	if e == nil || e.origin == nil || e.sql == nil || e.mongo == nil || e.aiReverse == nil || e.aiReverseCoordinator == nil || e.anchor != nil || e.reverseAnchor != nil {
		return fixedError("history_origin_anchor_rejected")
	}
	anchor, err := e.aiReverse.FreezeFreshAnchor(ctx, e.origin)
	if err != nil {
		return fixedError("history_origin_anchor_rejected")
	}
	e.reverseAnchor = anchor
	e.origin, e.sql, e.mongo, e.aiReverse, e.aiReverseCoordinator, e.anchor = nil, nil, nil, nil, nil, nil
	return nil
}

// Summary and legacy pointer checks do not establish actual snapshot freshness.
// Once the first graph is compacted, the anchor independently checks actual SQL
// ConnPool/CycleID and Mongo Lsid/Txn before its complete origin re-observation.
func compareEpochs(first, second *epochResult) error {
	if first == nil || second == nil || first.sql == second.sql || first.mongo == second.mongo || first.coordinator.CandidateSHA256 != second.coordinator.CandidateSHA256 || first.coordinator.CandidateCount != second.coordinator.CandidateCount || first.coordinator.LocallyQualifiedCount != second.coordinator.LocallyQualifiedCount || first.coordinator.BlockedLocalCount != second.coordinator.BlockedLocalCount || first.coordinator.ApprovedCopies != second.coordinator.ApprovedCopies || first.coordinator.SecondPassCopies != second.coordinator.SecondPassCopies || first.coordinator.ConsumedRecords != second.coordinator.ConsumedRecords || first.index.IndexSHA256 != second.index.IndexSHA256 || first.jointPages != second.jointPages || first.aiPages != second.aiPages || !reflect.DeepEqual(first.aiFacts, second.aiFacts) || !reflect.DeepEqual(first.aiReverseFacts, second.aiReverseFacts) || !reflect.DeepEqual(first.sqlFacts, second.sqlFacts) || !reflect.DeepEqual(first.mongoFacts, second.mongoFacts) || !reflect.DeepEqual(first.reasons, second.reasons) {
		return fixedError("history_independent_epoch_facts_changed")
	}
	return nil
}
func executePipeline(ctx context.Context, a *approvedInputs, d *historyDatabase) (r readiness, result error) {
	start := time.Now()
	r = emptyReadiness(a)
	defer func() { r.ElapsedMilliseconds = time.Since(start).Milliseconds() }()
	var first, second *epochResult
	if err := d.epoch(ctx, func(scope context.Context) error {
		var err error
		first, err = buildEpoch(scope, a, d)
		if err != nil {
			return err
		}
		return first.compactOrigin(scope)
	}); err != nil {
		return r, err
	}
	// The first SQL RRRO and Mongo snapshot have ended before the next starts.
	if err := d.epoch(ctx, func(scope context.Context) error {
		var e error
		second, e = buildEpoch(scope, a, d)
		if e != nil {
			return e
		}
		if e = compareEpochs(first, second); e != nil {
			return e
		}
		// Equality of exported diagnostics is not a freshness proof. This calls
		// the actual ended-old-Tx/new-ConnPool/cycle full 8+14 replay and binds
		// its four EOF readers to this independently authenticated coordinator.
		aiProof, e := first.recheckAIReverse(scope, second, a)
		if e != nil {
			return e
		}
		if a.rewind() != nil {
			return fixedError("history_asset_read_failed")
		}
		proof, e := first.reverseAnchor.RecheckOrigin(scope, aiProof, second.sql, second.mongo, second.aiReverseCoordinator, a.readers())
		if e != nil {
			return fixedError("history_actual_origin_independent_epoch_failed")
		}
		v := proof.Report()
		if !v.ActualOriginMatched || !v.IndependentEpochRechecked || !v.SourceFilesMatched || v.DropReady || v.CASAuthorized {
			return fixedError("history_actual_origin_independent_epoch_failed")
		}
		return a.verifyFullFiles(scope)
	}); err != nil {
		return r, err
	}
	r.CompletedReadOnlyPipeline = true
	r.IndependentEpochs = 2
	r.SourceFilesAndActualOriginsMatched = true
	r.WholeFourSourceCoverageComplete = true
	r.BusinessAndResponsibilityFactsUnchanged = true
	r.Sources = second.coordinator.SecondPassCopies
	r.WholeSourceIndexSHA256 = second.index.IndexSHA256
	r.CandidateSHA256 = second.coordinator.CandidateSHA256
	r.SQLCurrentFactsSHA256 = jsonHash(struct {
		SQL       stableSQLFacts
		AI        retirement.AIReadOnlyBindingSummary
		AIReverse stableAIReverseFacts
	}{second.sqlFacts, second.aiFacts, second.aiReverseFacts})
	r.MongoCurrentFactsSHA256 = jsonHash(second.mongoFacts)
	r.LocalCandidates = second.coordinator.CandidateCount
	r.LocallyQualified = second.coordinator.LocallyQualifiedCount
	r.BlockedLocal = second.coordinator.BlockedLocalCount
	r.JointEventPages = second.jointPages
	r.AIBlockedPages = second.aiPages
	r.SQLLedgerCount = len(second.sqlFacts.Ledgers)
	r.MongoCollectionCount = len(second.mongoFacts.Collections)
	r.SQLGlobal = sqlGlobalSummary{second.sqlFacts.Observed, second.sqlFacts.Related, second.sqlFacts.Outside, second.sqlFacts.Unknown, second.sqlFacts.Blocking, second.sqlFacts.SchemaCoverage}
	r.MongoGlobal = mongoGlobalSummary{second.mongoFacts.Rows, second.mongoFacts.ClassifiedRows, second.mongoFacts.ClassCounts, append([]string{}, second.mongoFacts.BlockingReasons...), append([]string{}, second.mongoFacts.CoverageGaps...)}
	r.AIReverseGlobal = aiReverseGlobalSummary{LedgerCount: len(second.aiReverseFacts.Ledgers), Rows: second.aiReverseFacts.Rows, Related: second.aiReverseFacts.Related, Outside: second.aiReverseFacts.Outside, Unknown: second.aiReverseFacts.Unknown, Blocking: second.aiReverseFacts.Blocking, OutsideActive: second.aiReverseFacts.OutsideActive, DataSHA256: second.aiReverseFacts.DataSHA256, SourceScopeSHA256: second.aiReverseFacts.SourceScopeSHA256, WholeLedgerEOF: true, IndependentEpochRechecked: true}
	r.BlockingReasons = second.reasons
	r.ErrorCategory = "none"
	r.RequiredAdapters = append(r.RequiredAdapters, second.coordinator.RequiredAdapters...)
	sort.Strings(r.RequiredAdapters)
	r.RequiredAdapters = uniqueStrings(r.RequiredAdapters)
	return r, nil
}
func stableAIReverseObservation(r retirement.AIReverseSummary) (stableAIReverseFacts, error) {
	// This is a body-free diagnostic serializer of an actual constructed
	// snapshot. It has no caller-facing qualification or write entrypoint.
	if r.MigrationVersion != 99 || !r.ActualReadOnlyRR || !r.WholeLedgerEOF || r.SourceAuthenticationRequired || len(r.Ledgers) != 14 || r.GlobalReverseQualified || r.CASAuthority || r.DropReady || !r.ExternalOriginRequired || !r.ExternalQSAIClosureRequired || !r.StoredWireAuthenticationRequired || !r.WriterFenceRequired {
		return stableAIReverseFacts{}, fixedError("history_ai_reverse_coverage_incomplete")
	}
	return stableAIReverseFacts{r.DatabaseIdentitySHA256, r.DataSHA256, r.BusinessAnchorsSHA256, r.SourceScopeSHA256, r.MigrationVersion, append([]retirement.AIReverseLedgerSummary(nil), r.Ledgers...), r.SourceCopies, r.Rows, r.Bytes, r.Related, r.OutsideRetirement, r.Unknown, r.Blocking, r.OutsideActive, append([]string(nil), r.BlockingReasons...)}, nil
}
func (first *epochResult) recheckAIReverse(ctx context.Context, second *epochResult, a *approvedInputs) (*retirement.AIReverseFreshProof, error) {
	if first == nil || second == nil || a == nil || first.reverseAnchor == nil || first.origin != nil || first.sql != nil || first.mongo != nil || first.aiReverse != nil || first.aiReverseCoordinator != nil || first.anchor != nil || second.aiReverse == nil || second.aiReverseCoordinator == nil || second.sql == nil {
		return nil, fixedError("history_ai_reverse_independent_epoch_failed")
	}
	if a.rewind() != nil {
		return nil, fixedError("history_asset_read_failed")
	}
	proof, err := first.reverseAnchor.RecheckFresh(ctx, second.sql, a.inventory.Migrations["mysql"], second.aiReverseCoordinator, a.copies())
	if err != nil || proof == nil {
		return nil, fixedError("history_ai_reverse_independent_epoch_failed")
	}
	return proof, nil
}

func uniqueStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		if len(out) == 0 || out[len(out)-1] != v {
			out = append(out, v)
		}
	}
	return out
}
