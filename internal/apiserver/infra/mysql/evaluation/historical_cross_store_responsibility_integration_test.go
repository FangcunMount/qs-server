//go:build integration

package evaluation

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	standard "github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	hostmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	eventpayload "github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
	"gorm.io/gorm"
)

func crossSQLNativeHeld(t *testing.T, db *gorm.DB, id string) {
	t.Helper()
	p := eventpayload.AnswerSheetSubmittedData{AnswerSheetID: "10042", QuestionnaireCode: "Q", QuestionnaireVersion: "1.0", OrgID: 7, TesteeID: 21, SubmittedAt: time.Date(2026, 10, 8, 1, 2, 3, 456000000, time.UTC)}
	row := cycleTestMessage(t, id, "answersheet.submitted", "AnswerSheet", "10042", p)
	if err := db.Exec("INSERT INTO retry_event_hold(event_id,message_id,org_id,provider,topic_name,channel_name,payload_json,original_delivery_attempt,blocked_reason,blocked_at,status,retry_disposition,replayed_at) VALUES(?,?,7,'nsq','synthetic-cross-topic','synthetic-cross-channel',?,1,'synthetic',UTC_TIMESTAMP(3),'replayed',NULL,UTC_TIMESTAMP(3))", id, "delivery-"+id, valueOrEmpty(row["payload"])).Error; err != nil {
		t.Fatal(err)
	}
}

func crossSQLNativeReplay(t *testing.T, db *gorm.DB, id string) {
	t.Helper()
	r := standard.ReplayRequest{OrgID: 7, RequestID: "native-cross-replay", Store: "mongo-domain-events", Reason: "native complete input", Targets: []standard.ReplayTarget{{EventID: id, ExpectedFailureCount: 3}, {EventID: "different-original", ExpectedFailureCount: 4}}}
	hash, err := r.Fingerprint()
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Exec("INSERT INTO qs_rm_replay_requests(org_id,request_id,store_name,reason,input_hash) VALUES(7,?,?,?,?)", []byte(r.RequestID), []byte(r.Store), r.Reason, hash[:]).Error; err != nil {
		t.Fatal(err)
	}
	for ordinal, target := range r.Targets {
		if err = db.Exec("INSERT INTO qs_rm_replay_items(org_id,request_id,ordinal,event_id,expected_failure_count,authorized,reason) VALUES(7,?,?,?,?,0,'not_found')", []byte(r.RequestID), ordinal, []byte(target.EventID), target.ExpectedFailureCount).Error; err != nil {
			t.Fatal(err)
		}
	}
}

func crossSQLNativePage(t *testing.T, db *gorm.DB, limits SQLCrossStoreLimits, fn func(context.Context, *SQLHistoricalCrossStoreCatalog, *SQLHistoricalCrossStorePage) error) error {
	t.Helper()
	return db.Transaction(func(tx *gorm.DB) error {
		ctx := hostmysql.WithTx(t.Context(), tx)
		uuid, name, err := historicalDatabase(tx)
		if err != nil {
			return err
		}
		cycle, err := PrepareSQLHistoricalResponsibilityCycle(ctx, sqlHistoricalIdentity(uuid, name), DefaultSQLResponsibilityLimits())
		if err != nil {
			return err
		}
		catalog, err := PrepareSQLHistoricalCrossStoreCatalog(ctx, cycle, limits)
		if err != nil {
			return err
		}
		batch, err := PrepareSQLHistoricalOwnerBatch(ctx, cycle, SQLHistoricalOwnerBatchRequest{AnswerSheetIDs: []uint64{10042}}, DefaultSQLHistoricalOwnerBatchLimits())
		if err != nil {
			return err
		}
		page, err := PrepareSQLHistoricalCrossStorePage(ctx, catalog, batch, SQLCrossStoreSelectors{EventIDs: []string{"native-cross-original"}, AssessmentIDs: []uint64{42}, OrganizationIDs: []uint64{7}, MongoOwners: []SQLCrossStoreOwnerReference{{Kind: "AnswerSheet", ID: "10042"}}})
		if err != nil {
			return err
		}
		return fn(ctx, catalog, page)
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
}

func TestSQLCrossStoreNativeActualKeysRawRowsWholeReplayAndZeroLookupSQL(t *testing.T) {
	db := openHistoricalReferencesDB(t)
	insertHistoricalAssessment(t, db, 42)
	crossSQLNativeHeld(t, db, "native-cross-original")
	crossSQLNativeReplay(t, db, "native-cross-original")
	if err := crossSQLNativePage(t, db, DefaultSQLCrossStoreLimits(), func(ctx context.Context, c *SQLHistoricalCrossStoreCatalog, p *SQLHistoricalCrossStorePage) error {
		if !c.Report().Complete || c.Report().Keys != 4 || p.Report().Rows != 4 || !p.Report().Complete || p.Report().DropReady {
			return ErrSQLCrossStoreConflict
		}
		tx, _ := hostmysql.RequireTx(ctx)
		var before, after uint64
		var metric string
		if err := tx.Raw("SHOW SESSION STATUS LIKE 'Com_select'").Row().Scan(&metric, &before); err != nil {
			return err
		}
		for i := 0; i < 10000; i++ {
			rows, err := p.Lookup("native-cross-original", 42, 7)
			if err != nil {
				return err
			}
			if len(rows) != 2 {
				return ErrSQLCrossStoreConflict
			}
			for _, row := range rows {
				if row.Observation.Store == "retry_event_hold" && (row.Inner == nil || row.LegacyContentSHA256 == "" || row.InnerDataSHA256 == "" || row.Observation.RowSHA256 == row.LegacyContentSHA256) {
					return ErrSQLCrossStoreConflict
				}
				if row.Replay != nil && (!row.Replay.FingerprintVerified || len(row.Replay.Items) != 2) {
					return ErrSQLCrossStoreConflict
				}
			}
			if i == 0 {
				rows[0].Observation.Reasons = append(rows[0].Observation.Reasons, "edited")
				if rows[0].Inner != nil {
					rows[0].Inner.ID = "edited"
				}
				if rows[1].Replay != nil {
					rows[1].Replay.Items[0].EventID = "edited"
				}
			}
		}
		if err := tx.Raw("SHOW SESSION STATUS LIKE 'Com_select'").Row().Scan(&metric, &after); err != nil {
			return err
		}
		if after != before {
			return ErrSQLCrossStoreConflict
		}
		r := c.Report()
		if len(r.Ledgers) != 8 {
			return ErrSQLCrossStoreConflict
		}
		for i, ledger := range r.Ledgers {
			if ledger.SchemaSHA256 == "" || ledger.PrimaryKeySHA256 == "" || ledger.KeysSHA256 == "" || ledger.Keys > 0 && (ledger.FirstObservedKeySHA256 == "" || ledger.LastObservedKeySHA256 != ledger.FixedUpperKeySHA256) {
				return ErrSQLCrossStoreConflict
			}
			t.Logf("server_ordered_primary_bound ledger=%s keys=%d queries=%d first_sha256=%s upper_sha256=%s last_sha256=%s keys_sha256=%s", ledger.Store, ledger.Keys, ledger.Queries, ledger.FirstObservedKeySHA256, ledger.FixedUpperKeySHA256, ledger.LastObservedKeySHA256, ledger.KeysSHA256)
			r.Ledgers[i].Store = "edited"
		}
		if c.Report().Ledgers[0].Store == "edited" {
			return ErrSQLCrossStoreConflict
		}
		t.Log("actual SQL8 PRIMARY catalog and full-row page binding; ordered replay whole input; cached_lookups=10000 new_sql_selects=0")
		return p.ValidateBorrowedSnapshot(ctx)
	}); err != nil {
		t.Fatal(err)
	}
}

func TestSQLCrossStoreNativeBudgetsAndWrongActualTransaction(t *testing.T) {
	for _, name := range []string{"keys_budget", "key_bytes_budget", "rows_budget", "page_bytes_budget", "completed_transaction"} {
		t.Run(name, func(t *testing.T) {
			db := openHistoricalReferencesDB(t)
			insertHistoricalAssessment(t, db, 42)
			crossSQLNativeHeld(t, db, "native-cross-original")
			crossSQLNativeReplay(t, db, "native-cross-original")
			limits := DefaultSQLCrossStoreLimits()
			switch name {
			case "keys_budget":
				limits.MaxKeys = 1
			case "key_bytes_budget":
				limits.MaxKeyRetainedBytes = 1
			case "rows_budget":
				limits.MaxPageRows = 1
			case "page_bytes_budget":
				limits.MaxPageBytes = 1
			}
			var saved *SQLHistoricalCrossStorePage
			err := crossSQLNativePage(t, db, limits, func(_ context.Context, _ *SQLHistoricalCrossStoreCatalog, p *SQLHistoricalCrossStorePage) error {
				saved = p
				return nil
			})
			if name == "completed_transaction" {
				if err != nil {
					t.Fatal(err)
				}
				if err := saved.ValidateBorrowedSnapshot(t.Context()); err == nil {
					t.Fatal("completed host tx accepted")
				}
				return
			}
			if err == nil || !errors.Is(err, ErrSQLCrossStoreBounds) && !errors.Is(err, ErrSQLCrossStoreInvalid) {
				t.Fatal("incomplete budget returned a complete cache", err)
			}
		})
	}
}

func TestSQLCrossStoreNativeUnsupportedSchemaEvenWhenEmpty(t *testing.T) {
	for _, name := range []string{"unknown_future_column", "missing_existing_column"} {
		t.Run(name, func(t *testing.T) {
			db := openHistoricalReferencesDB(t)
			query := "ALTER TABLE retry_event_hold ADD COLUMN future_execution_responsibility BIGINT NULL"
			if name == "missing_existing_column" {
				query = "ALTER TABLE retry_event_hold DROP COLUMN blocked_reason"
			}
			if err := db.Exec(query).Error; err != nil {
				t.Fatal(err)
			}
			err := crossSQLNativePage(t, db, DefaultSQLCrossStoreLimits(), func(context.Context, *SQLHistoricalCrossStoreCatalog, *SQLHistoricalCrossStorePage) error {
				return ErrSQLCrossStoreConflict
			})
			if !errors.Is(err, ErrSQLCrossStoreSchema) {
				t.Fatal("empty unsupported schema was treated as responsibility-free", err)
			}
		})
	}
}

func TestSQLCrossStoreNativeExactCycleHashRequired(t *testing.T) {
	db := openHistoricalReferencesDB(t)
	insertHistoricalAssessment(t, db, 42)
	crossSQLNativeHeld(t, db, "native-cross-original")
	err := db.Transaction(func(tx *gorm.DB) error {
		ctx := hostmysql.WithTx(t.Context(), tx)
		uuid, name, err := historicalDatabase(tx)
		if err != nil {
			return err
		}
		cycle, err := PrepareSQLHistoricalResponsibilityCycle(ctx, sqlHistoricalIdentity(uuid, name), DefaultSQLResponsibilityLimits())
		if err != nil {
			return err
		}
		catalog, err := PrepareSQLHistoricalCrossStoreCatalog(ctx, cycle, DefaultSQLCrossStoreLimits())
		if err != nil {
			return err
		}
		batch, err := PrepareSQLHistoricalOwnerBatch(ctx, cycle, SQLHistoricalOwnerBatchRequest{AnswerSheetIDs: []uint64{10042}}, DefaultSQLHistoricalOwnerBatchLimits())
		if err != nil {
			return err
		}
		// A corrupted compact baseline cannot authorize a different actual row.
		cycle.observations[0].RowSHA256 = "invalid-baseline"
		_, err = PrepareSQLHistoricalCrossStorePage(ctx, catalog, batch, SQLCrossStoreSelectors{EventIDs: []string{"native-cross-original"}, OrganizationIDs: []uint64{7}})
		if !errors.Is(err, ErrSQLCrossStoreConflict) {
			return ErrSQLCrossStoreInvalid
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
}
