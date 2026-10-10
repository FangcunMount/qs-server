package main

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"reflect"
	"strconv"
	"time"

	sqlevaluation "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	retirement "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirement"
	hostmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// This replaces the initial two full reads of the bounded write host. It must
// replace the old long-transaction evidence writer. It returns pure
// input only: the root host must independently qualify each fresh component.
type historyInitialInputs struct {
	sources      *retirement.HistoricalSourceInputPair
	ai           *retirement.AIHistoricalInputPair
	index        *retirement.WholeSourceJointIndex
	components   *retirement.HistoricalCASComponents
	ownerSpool   *sqlevaluation.SQLHistoricalCASSpool
	sourceEpochs [2]*retirement.HistoricalSourceInputEpoch
	aiEpochs     [2]*retirement.AIHistoricalInputEpoch
	sqlFacts     [2]stableSQLFacts
	mongoFacts   [2]retirement.MongoSnapshotInputSummary
	files        [7]*os.File
}

func historyInitialInputNames() [6]string {
	return [6]string{"input-mongo-1.private.bin", "input-source-1.private.bin", "input-ai-1.private.bin", "input-mongo-2.private.bin", "input-source-2.private.bin", "input-ai-2.private.bin"}
}

const historyInputOwnerSQLName = "input-owner-sql.private.bin"

func (input *historyInitialInputs) close() error {
	if input == nil {
		return nil
	}
	var result error
	for i, f := range input.files {
		if f != nil {
			if f.Close() != nil {
				result = fixedError("history_write_private_close_failed")
			}
			input.files[i] = nil
		}
	}
	return result
}

// One genuine SQL RRRO transaction and one nontransaction snapshot session.
// No Mongo StartTransaction, commit, abort, replacement session or retry exists
// on this input path. The host rolls SQL back and ends the session exactly once.
func (d *historyDatabase) snapshotInputScope(ctx context.Context, fn func(context.Context) error) (result error) {
	if d == nil || d.sql == nil || d.mongoClient == nil || d.mongo == nil || ctx == nil || ctx.Err() != nil || fn == nil {
		return fixedError("history_host_epoch_rejected")
	}
	if err := d.validateNamespaceAnchor(ctx); err != nil {
		return err
	}
	defer func() {
		if err := d.validateNamespaceAnchor(ctx); result == nil && err != nil {
			result = err
		}
	}()
	s, err := d.mongoClient.StartSession(options.Session().SetSnapshot(true).SetCausalConsistency(false))
	if err != nil {
		return fixedError("history_host_epoch_rejected")
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		s.EndSession(cleanup)
		cancel()
	}()
	tx := d.sql.WithContext(ctx).Begin(&sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if tx.Error != nil {
		return fixedError("history_host_epoch_rejected")
	}
	defer func() {
		if err := tx.Rollback().Error; err != nil && !errors.Is(err, sql.ErrTxDone) && result == nil {
			result = fixedError("history_host_sql_rollback_failed")
		}
	}()
	paired := mongo.NewSessionContext(hostmysql.WithTx(ctx, tx), s)
	checkHead := func() error {
		q, cancel := context.WithTimeout(paired, 30*time.Second)
		defer cancel()
		var rows []struct {
			Version string
			Dirty   string
		}
		if tx.WithContext(q).Raw("SELECT CAST(version AS BINARY) AS version,CAST(dirty AS BINARY) AS dirty FROM schema_migrations ORDER BY version LIMIT 2").Scan(&rows).Error != nil || len(rows) != 1 || rows[0].Dirty != "0" || rows[0].Version != strconv.FormatUint(d.sqlHead, 10) {
			return fixedError("history_migration_binding_rejected")
		}
		return nil
	}
	if err = checkHead(); err != nil {
		return err
	}
	if err = fn(paired); err != nil {
		return err
	}
	return checkHead()
}

func captureHistoryInitialInputs(ctx context.Context, a *approvedInputs, d *historyDatabase, journal *historyWriteJournal) (result *historyInitialInputs, err error) {
	if ctx == nil || ctx.Err() != nil || a == nil || d == nil || journal == nil || !journal.valid() || journal.binding.SourceSHA != a.request.SourceSHA || journal.binding.OperationID != a.request.OperationID || journal.run != a.request.RunID || a.verifyFullFiles(ctx) != nil {
		return nil, fixedError("history_write_input_rejected")
	}
	input := &historyInitialInputs{}
	defer func() {
		if err != nil {
			journal.unknown = true
			_ = input.close()
		}
	}()
	for i, name := range historyInitialInputNames() {
		input.files[i], err = journal.create(name)
		if err != nil {
			return nil, err
		}
	}
	input.files[6], err = journal.create(historyInputOwnerSQLName)
	if err != nil {
		return nil, err
	}
	input.ownerSpool, err = sqlevaluation.NewSQLHistoricalCASSpool(input.files[6], 16<<30, 128<<20)
	if err != nil {
		return nil, fixedError("history_write_spool_failed")
	}
	// Authenticate the source copies once before the native rounds. This
	// existing compact index has its original reservation/duration limits;
	// it is not a measured RSS guarantee or any future writer authority.
	if a.rewind() != nil {
		return nil, fixedError("history_asset_read_failed")
	}
	authenticated, err := retirement.VerifySourceCopies(ctx, a.copies())
	if err != nil {
		return nil, fixedError("history_source_authentication_failed")
	}
	if a.rewind() != nil {
		return nil, fixedError("history_asset_read_failed")
	}
	binding, err := retirement.BindOriginCopies(ctx, authenticated, a.originCopies(), retirement.DefaultSourceOriginLimits())
	if err != nil {
		return nil, fixedError("history_source_origin_binding_failed")
	}
	recipe, err := retirement.FreezeHistoricalSourceInputRecipe(ctx, binding)
	if err != nil {
		return nil, fixedError("history_source_origin_binding_failed")
	}
	indexLimits := retirement.DefaultWholeSourceJointLimits()
	indexLimits.MaxRelatedSources = 512
	input.index, err = retirement.PrepareHistoricalSourceInputIndex(ctx, journal.binding, recipe, a.jointCopies(), indexLimits)
	if err != nil {
		return nil, fixedError("history_complete_source_index_failed")
	}
	for round := range 2 {
		err = d.snapshotInputScope(ctx, func(scope context.Context) error {
			sqlLimits := sqlevaluation.DefaultSQLResponsibilityLimits()
			sqlLimits.MaxRetainedBytes = 128 << 20
			sqlInput, e := retirement.PrepareSQLResponsibilitySnapshot(scope, a.inventory.Identities["mysql"], sqlLimits)
			if e != nil {
				return fixedError("history_global_sql_scan_failed")
			}
			r := sqlInput.Report()
			if len(r.Ledgers) != 8 || !r.ActualTransactionReadOnlyRR || r.CompletedAt.IsZero() {
				return fixedError("history_global_coverage_incomplete")
			}
			input.sqlFacts[round] = stableSQLFacts{r.DatabaseIdentitySHA256, r.BusinessAnchorsSHA256, r.SchemaCoverage, r.Ledgers, r.Observed, r.RetirementRelated, r.OutsideRetirement, r.Unknown, r.Blocking}
			mongoLimits := retirement.MongoResponsibilityLimits{PageRows: 512, MaxRows: 2_000_000, MaxBytes: 8 << 30, MaxPages: 2_000_000, MaxGraphEntries: 4_000_000, MaxGraphBytes: 128 << 20, MaxDuration: 5 * time.Minute}
			mongoInput, e := retirement.PrepareMongoSnapshotInputEpoch(scope, d.mongo, d.mongoConfig, retirement.MongoSnapshotInputLimits{Scan: mongoLimits, MaxDuration: 25 * time.Minute}, input.files[round*3])
			if e != nil {
				return fixedError("history_global_mongo_scan_failed")
			}
			input.mongoFacts[round] = mongoInput.Summary()
			if !input.mongoFacts[round].CompleteInput || len(input.mongoFacts[round].Collections) != 11 {
				return fixedError("history_global_coverage_incomplete")
			}
			input.sourceEpochs[round], e = retirement.PrepareHistoricalSourceInputEpoch(scope, recipe, sqlInput, mongoInput, input.files[round*3+1], 25*time.Minute)
			if e != nil {
				return fixedError("history_actual_source_origin_failed")
			}
			if a.rewind() != nil {
				return fixedError("history_asset_read_failed")
			}
			aiLimits := retirement.DefaultAIReverseLimits()
			aiLimits.MaxRetainedBytes = 128 << 20
			input.aiEpochs[round], e = retirement.PrepareAIHistoricalInputEpoch(scope, nil, input.sourceEpochs[round], a.copies(), input.files[round*3+2], aiLimits)
			if e != nil {
				return fixedError("history_ai_reverse_source_scope_failed")
			}
			if round == 1 {
				// Freeze ownership while this actual second RRRO/snapshot is
				// still alive. The returned recipes grant no write authority.
				if !reflect.DeepEqual(input.sqlFacts[0], input.sqlFacts[1]) {
					return fixedError("history_independent_epoch_facts_changed")
				}
				firstMongo, secondMongo := input.mongoFacts[0], input.mongoFacts[1]
				firstMongo.NativeEpochSHA256, secondMongo.NativeEpochSHA256 = "", ""
				if !reflect.DeepEqual(firstMongo, secondMongo) {
					return fixedError("history_independent_epoch_facts_changed")
				}
				input.sources, e = retirement.CompareIndependentHistoricalSourceInputs(scope, input.sourceEpochs[0], input.sourceEpochs[1])
				if e != nil {
					return fixedError("history_independent_epoch_facts_changed")
				}
				input.ai, e = retirement.CompareIndependentAIHistoricalInputs(scope, input.aiEpochs[0], input.aiEpochs[1], input.sources)
				if e != nil {
					return fixedError("history_independent_epoch_facts_changed")
				}
				input.components, e = retirement.PlanHistoricalSourceOwnerComponents(scope, input.sources, input.index, sqlInput, mongoInput, input.ownerSpool, retirement.DefaultHistoricalCASComponentLimits())
				if e != nil {
					return fixedError("history_component_owner_planning_failed")
				}
			}
			return input.sourceEpochs[round].StopCapture(scope)
		})
		if err != nil {
			return nil, err
		}
		if journal.record(ctx, "initial_input_epoch_frozen", round, 0, nil) != nil {
			return nil, fixedError("history_write_journal_unknown")
		}
	}
	if input.sources == nil || input.ai == nil || input.components == nil || input.index.ReleaseInputAuthentication(ctx, input.sources) != nil || input.sources.ReleaseCaptureIndex(ctx) != nil || input.components.ValidateInputSources(ctx, input.sources) != nil || input.ai.ValidateFrozen(ctx) != nil {
		return nil, fixedError("history_independent_epoch_facts_changed")
	}
	if journal.record(ctx, "initial_inputs_matched", -1, 0, nil) != nil {
		return nil, fixedError("history_write_journal_unknown")
	}
	return input, nil
}
